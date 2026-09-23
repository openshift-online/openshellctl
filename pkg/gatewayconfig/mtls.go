package gatewayconfig

import "io/fs"

// TLSMaterial locates a gateway's mTLS files under gateways/<name>/mtls/. Upstream
// uses ca.crt / tls.crt / tls.key (mtls.rs:30-38). Present is true when at least
// ca.crt exists; the full triple (all three) is required for client-cert auth,
// ca.crt alone means "verify server with this CA", none means system roots
// (tls.rs:423-449).
type TLSMaterial struct {
	CAFile   string
	CertFile string
	KeyFile  string
	Present  bool

	caExists   bool
	certExists bool
	keyExists  bool
}

// HasClientCert reports whether the full client-cert triple is available.
func (m TLSMaterial) HasClientCert() bool {
	return m.caExists && m.certExists && m.keyExists
}

// TLSMaterialFor resolves the mtls/ file paths for a resolved gateway. Paths are
// relative to the gateway sub-FS (r.FS). Present is true iff ca.crt exists.
// CertFile/KeyFile are only set when both exist (full client-cert triple);
// ca.crt alone means "verify server with this CA" (no client cert).
func TLSMaterialFor(r *Resolved) TLSMaterial {
	var m TLSMaterial
	if r == nil || r.FS == nil {
		return m
	}
	const (
		caPath   = "mtls/ca.crt"
		certPath = "mtls/tls.crt"
		keyPath  = "mtls/tls.key"
	)
	m.caExists = fileExists(r.FS, caPath)
	if !m.caExists {
		return TLSMaterial{}
	}
	m.CAFile = caPath
	m.Present = true
	m.certExists = fileExists(r.FS, certPath)
	m.keyExists = fileExists(r.FS, keyPath)
	if m.HasClientCert() {
		m.CertFile = certPath
		m.KeyFile = keyPath
	}
	return m
}

func fileExists(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
