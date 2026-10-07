package gatewayconfig

import (
	"errors"
	"testing"
)

func TestSetActive_WritesFile(t *testing.T) {
	w := newMemWriter()
	if err := SetActive(w, "rosa"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if string(w.files["active_gateway"]) != "rosa" {
		t.Errorf("active_gateway = %q, want %q", w.files["active_gateway"], "rosa")
	}
}

func TestSetActive_InvalidName(t *testing.T) {
	w := newMemWriter()
	err := SetActive(w, "has/slash")
	var invalid *InvalidGatewayNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidGatewayNameError", err)
	}
}

func TestClearActiveIfMatches_Matching(t *testing.T) {
	w := newMemWriter()
	_ = SetActive(w, "rosa")
	if err := ClearActiveIfMatches(w, "rosa"); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.files["active_gateway"]; ok {
		t.Error("active_gateway should be cleared when it matches")
	}
}

func TestClearActiveIfMatches_NonMatching(t *testing.T) {
	w := newMemWriter()
	_ = SetActive(w, "rosa")
	if err := ClearActiveIfMatches(w, "other"); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.files["active_gateway"]; !ok {
		t.Error("active_gateway should NOT be cleared when it doesn't match")
	}
}

func TestClearActiveIfMatches_MissingFile(t *testing.T) {
	w := newMemWriter()
	if err := ClearActiveIfMatches(w, "rosa"); err != nil {
		t.Errorf("clearing a missing active_gateway should be a no-op, got %v", err)
	}
}

func TestRemoveGateway_RemovesMetadataAndToken(t *testing.T) {
	w := newMemWriter()
	_ = w.WriteFile("gateways/rosa/metadata.json", []byte("{}"), 0o600)
	_ = w.WriteFile("gateways/rosa/oidc_token.json", []byte("{}"), 0o600)
	// unrelated file in another gateway must survive
	_ = w.WriteFile("gateways/other/metadata.json", []byte("{}"), 0o600)

	if err := RemoveGateway(w, "rosa"); err != nil {
		t.Fatalf("RemoveGateway: %v", err)
	}
	for rel := range w.files {
		if rel == "gateways/other/metadata.json" {
			continue
		}
		t.Errorf("file %q should have been removed", rel)
	}
	if _, ok := w.files["gateways/other/metadata.json"]; !ok {
		t.Error("unrelated gateway's files must survive")
	}
}

// TestRemoveGateway_PreservesMTLSAndLastSandbox confirms RemoveGateway never
// deletes mtls/ material or last_sandbox: mTLS certs/keys are typically
// admin-issued and not recreatable by this CLI, so removing a registration
// (e.g. to fix a typo'd endpoint and re-add it) must never destroy them.
// last_sandbox is harmless to leave behind (it's just a pointer, trivially
// stale) and is kept for the same reason: only the two files this CLI itself
// creates and can recreate (metadata.json, oidc_token.json) are removed.
func TestRemoveGateway_PreservesMTLSAndLastSandbox(t *testing.T) {
	w := newMemWriter()
	_ = w.WriteFile("gateways/rosa/metadata.json", []byte("{}"), 0o600)
	_ = w.WriteFile("gateways/rosa/last_sandbox", []byte("default\nsb"), 0o600)
	_ = w.WriteFile("gateways/rosa/mtls/ca.crt", []byte("ca"), 0o600)
	_ = w.WriteFile("gateways/rosa/mtls/tls.crt", []byte("cert"), 0o600)
	_ = w.WriteFile("gateways/rosa/mtls/tls.key", []byte("key"), 0o600)

	if err := RemoveGateway(w, "rosa"); err != nil {
		t.Fatalf("RemoveGateway: %v", err)
	}
	for _, rel := range []string{
		"gateways/rosa/last_sandbox",
		"gateways/rosa/mtls/ca.crt",
		"gateways/rosa/mtls/tls.crt",
		"gateways/rosa/mtls/tls.key",
	} {
		if _, ok := w.files[rel]; !ok {
			t.Errorf("%q should survive RemoveGateway (not recreatable by this CLI)", rel)
		}
	}
}

func TestRemoveGateway_MissingFilesAreNoOp(t *testing.T) {
	w := newMemWriter()
	if err := RemoveGateway(w, "rosa"); err != nil {
		t.Errorf("removing a never-registered gateway should be a no-op, got %v", err)
	}
}

func TestRemoveGateway_InvalidName(t *testing.T) {
	w := newMemWriter()
	err := RemoveGateway(w, "has/slash")
	var invalid *InvalidGatewayNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidGatewayNameError", err)
	}
}

func TestLogout_RemovesOnlyToken(t *testing.T) {
	w := newMemWriter()
	_ = w.WriteFile("gateways/rosa/metadata.json", []byte("{}"), 0o600)
	_ = w.WriteFile("gateways/rosa/oidc_token.json", []byte("{}"), 0o600)

	if err := Logout(w, "rosa"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, ok := w.files["gateways/rosa/oidc_token.json"]; ok {
		t.Error("oidc_token.json should be removed by Logout")
	}
	if _, ok := w.files["gateways/rosa/metadata.json"]; !ok {
		t.Error("Logout must not remove metadata.json (registration stays)")
	}
}

func TestLogout_MissingTokenIsNoOp(t *testing.T) {
	w := newMemWriter()
	if err := Logout(w, "rosa"); err != nil {
		t.Errorf("logging out with no token present should be a no-op, got %v", err)
	}
}

func TestLogout_InvalidName(t *testing.T) {
	w := newMemWriter()
	err := Logout(w, "has/slash")
	var invalid *InvalidGatewayNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidGatewayNameError", err)
	}
}
