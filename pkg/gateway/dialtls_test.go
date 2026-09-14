package gateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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
