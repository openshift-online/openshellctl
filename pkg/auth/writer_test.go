package auth

import (
	"io/fs"
	"testing"
	"time"
)

type recordWriter struct {
	path string
	data []byte
	perm fs.FileMode
}

func (w *recordWriter) WriteFile(path string, data []byte, perm fs.FileMode) error {
	w.path = path
	w.data = append([]byte(nil), data...)
	w.perm = perm
	return nil
}

func TestWriteBundle(t *testing.T) {
	w := &recordWriter{}
	tok := &Token{
		AccessToken: "abc",
		Issuer:      "https://i",
		ClientID:    "openshell-cli",
		Expiry:      time.Unix(1700000000, 0),
	}
	if err := WriteBundle(w, tok); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	if w.path != "oidc_token.json" {
		t.Errorf("path = %q", w.path)
	}
	if w.perm != 0o600 {
		t.Errorf("perm = %o, want 600", w.perm)
	}
	// The written bytes must parse back and carry no refresh token.
	got, err := ParseDiskBundle(w.data)
	if err != nil {
		t.Fatalf("written bundle does not parse: %v\n%s", err, w.data)
	}
	if got.AccessToken != "abc" || got.Issuer != "https://i" || got.ClientID != "openshell-cli" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.RefreshToken != nil {
		t.Error("client-credentials bundle must not carry a refresh_token")
	}
	if got.ExpiresAt == nil || *got.ExpiresAt != 1700000000 {
		t.Errorf("expires_at = %v", got.ExpiresAt)
	}
}

func TestWriteBundle_ZeroExpiryOmitted(t *testing.T) {
	w := &recordWriter{}
	tok := &Token{AccessToken: "abc", Issuer: "https://i", ClientID: "c"}
	if err := WriteBundle(w, tok); err != nil {
		t.Fatal(err)
	}
	got, _ := ParseDiskBundle(w.data)
	if got.ExpiresAt != nil {
		t.Errorf("zero expiry should be omitted, got %v", got.ExpiresAt)
	}
}
