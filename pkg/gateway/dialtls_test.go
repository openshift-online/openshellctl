package gateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// writeSelfSigned writes a self-signed cert+key+CA (all the same cert) into dir
// and returns the mtls-relative material rooted at dir.
func writeSelfSigned(t *testing.T, dir string) gatewayconfig.TLSMaterial {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	mtls := filepath.Join(dir, "mtls")
	if err := os.MkdirAll(mtls, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(mtls, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("ca.crt", certPEM)
	writeFile("tls.crt", certPEM)
	writeFile("tls.key", keyPEM)

	return gatewayconfig.TLSMaterial{
		Present:  true,
		CAFile:   "mtls/ca.crt",
		CertFile: "mtls/tls.crt",
		KeyFile:  "mtls/tls.key",
	}
}

func TestBuildTLSConfig_FullTriple(t *testing.T) {
	dir := t.TempDir()
	writeSelfSigned(t, dir)
	// Resolve the material (with existence flags) via TLSMaterialFor over a DirFS.
	resolved := &gatewayconfig.Resolved{FS: os.DirFS(dir)}
	mat := gatewayconfig.TLSMaterialFor(resolved)

	cfg, err := buildTLSConfig(DialConfig{TLS: mat, TLSRoot: dir})
	if err != nil {
		t.Fatalf("buildTLSConfig: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set from ca.crt")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("client certs = %d, want 1", len(cfg.Certificates))
	}
	if cfg.MinVersion == 0 {
		t.Error("MinVersion should be set")
	}
}

func TestBuildTLSConfig_BadCAErrors(t *testing.T) {
	mat := gatewayconfig.TLSMaterial{Present: true, CAFile: "/nonexistent/ca.crt"}
	if _, err := buildTLSConfig(DialConfig{TLS: mat}); err == nil {
		t.Error("expected an error reading a missing CA file")
	}
}

func TestBuildTLSConfig_Insecure(t *testing.T) {
	cfg, err := buildTLSConfig(DialConfig{Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be set")
	}
}

func TestDial_TLSWithMaterial(t *testing.T) {
	dir := t.TempDir()
	_ = writeSelfSigned(t, dir)
	resolved := &gatewayconfig.Resolved{FS: os.DirFS(dir)}
	mat := gatewayconfig.TLSMaterialFor(resolved)

	conn, err := Dial(DialConfig{Endpoint: "https://gw:443", TLS: mat, TLSRoot: dir})
	if err != nil {
		t.Fatalf("Dial with TLS material: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}

func TestBuildTLSConfig_ReadsFromConfigTree_NotCWD(t *testing.T) {
	configDir := t.TempDir()
	writeSelfSigned(t, configDir)
	resolved := &gatewayconfig.Resolved{Dir: configDir, FS: os.DirFS(configDir)}
	mat := gatewayconfig.TLSMaterialFor(resolved)

	elsewhere := t.TempDir()
	t.Chdir(elsewhere)

	cfg, err := buildTLSConfig(DialConfig{TLS: mat, TLSRoot: resolved.Dir})
	if err != nil {
		t.Fatalf("buildTLSConfig from config tree: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be populated from the config tree CA")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("client certs = %d, want 1", len(cfg.Certificates))
	}
}

func TestBuildTLSConfig_CWD_MTLS_NotConsulted(t *testing.T) {
	configDir := t.TempDir()
	writeSelfSigned(t, configDir)
	resolved := &gatewayconfig.Resolved{Dir: configDir, FS: os.DirFS(configDir)}
	mat := gatewayconfig.TLSMaterialFor(resolved)

	cwd := t.TempDir()
	writeSelfSigned(t, cwd)
	t.Chdir(cwd)

	cfg, err := buildTLSConfig(DialConfig{TLS: mat, TLSRoot: resolved.Dir})
	if err != nil {
		t.Fatalf("buildTLSConfig: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs should be set from config tree")
	}

	// Verify the CA was loaded from configDir, not CWD: the CWD's
	// self-signed cert (different key) must fail verification against
	// the pool that buildTLSConfig built from the config tree.
	cwdCA, err := os.ReadFile(filepath.Join(cwd, "mtls", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	// The CWD cert must not be in the pool (different key).
	cwdCert, _ := pem.Decode(cwdCA)
	parsed, err := x509.ParseCertificate(cwdCert.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	_, verifyErr := parsed.Verify(x509.VerifyOptions{Roots: cfg.RootCAs})
	if verifyErr == nil {
		t.Error("CWD CA should NOT be trusted by the built TLS config — config tree CA expected")
	}
}

func TestBuildTLSConfig_RejectsRelativePaths(t *testing.T) {
	mat := gatewayconfig.TLSMaterial{
		Present:  true,
		CAFile:   "mtls/ca.crt",
		CertFile: "mtls/tls.crt",
		KeyFile:  "mtls/tls.key",
	}
	_, err := buildTLSConfig(DialConfig{TLS: mat, TLSRoot: ""})
	if err == nil {
		t.Fatal("expected error for relative TLS paths with empty TLSRoot")
	}
	if !errors.Is(err, ErrRelativeTLSPath) {
		t.Errorf("err = %v, want ErrRelativeTLSPath", err)
	}
}
