package gatewayconfig

import (
	"testing"
	"testing/fstest"
)

func TestTLSMaterialFor_FullTriple(t *testing.T) {
	r := &Resolved{FS: fstest.MapFS{
		"mtls/ca.crt":  {Data: []byte("ca")},
		"mtls/tls.crt": {Data: []byte("cert")},
		"mtls/tls.key": {Data: []byte("key")},
	}}
	m := TLSMaterialFor(r)
	if !m.Present {
		t.Fatal("Present should be true when ca.crt exists")
	}
	if !m.HasClientCert() {
		t.Error("HasClientCert should be true for the full triple")
	}
	if m.CAFile != "mtls/ca.crt" || m.CertFile != "mtls/tls.crt" || m.KeyFile != "mtls/tls.key" {
		t.Errorf("unexpected paths: %+v", m)
	}
}

func TestTLSMaterialFor_CAOnly(t *testing.T) {
	r := &Resolved{FS: fstest.MapFS{"mtls/ca.crt": {Data: []byte("ca")}}}
	m := TLSMaterialFor(r)
	if !m.Present {
		t.Fatal("Present should be true with ca.crt only")
	}
	if m.HasClientCert() {
		t.Error("HasClientCert should be false without cert+key")
	}
}

func TestTLSMaterialFor_None(t *testing.T) {
	r := &Resolved{FS: fstest.MapFS{}}
	m := TLSMaterialFor(r)
	if m.Present {
		t.Error("Present should be false with no ca.crt (system roots case)")
	}
	if m.CAFile != "" {
		t.Errorf("no material should be reported, got %+v", m)
	}
}

func TestTLSMaterialFor_NilResolved(t *testing.T) {
	if m := TLSMaterialFor(nil); m.Present {
		t.Error("nil resolved should yield no material")
	}
}
