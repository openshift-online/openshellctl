package auth

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestParseDiskBundle_Valid(t *testing.T) {
	raw := []byte(`{"access_token":"tok","refresh_token":"r","expires_at":2000,"issuer":"https://i","client_id":"openshell-cli"}`)
	db, err := ParseDiskBundle(raw)
	if err != nil {
		t.Fatalf("ParseDiskBundle: %v", err)
	}
	if db.AccessToken != "tok" || db.Issuer != "https://i" || db.ClientID != "openshell-cli" {
		t.Errorf("unexpected bundle: %+v", db)
	}
	if db.RefreshToken == nil || *db.RefreshToken != "r" {
		t.Errorf("refresh_token = %v", db.RefreshToken)
	}
	if db.ExpiresAt == nil || *db.ExpiresAt != 2000 {
		t.Errorf("expires_at = %v", db.ExpiresAt)
	}
}

func TestParseDiskBundle_MissingIssuer(t *testing.T) {
	raw := []byte(`{"access_token":"tok","client_id":"c"}`)
	_, err := ParseDiskBundle(raw)
	var invalid *ErrBundleInvalid
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want ErrBundleInvalid", err)
	}
}

func TestDiskBundle_MarshalGolden(t *testing.T) {
	r := "refresh-xyz"
	exp := int64(1700000000)
	b := DiskBundle{
		AccessToken:  "abc",
		RefreshToken: &r,
		ExpiresAt:    &exp,
		Issuer:       "https://issuer.example.com/realms/openshell",
		ClientID:     "openshell-cli",
	}
	got, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// serde_json::to_string_pretty layout: 2-space indent, keys in struct order,
	// no trailing newline, no HTML escaping.
	want := `{
  "access_token": "abc",
  "refresh_token": "refresh-xyz",
  "expires_at": 1700000000,
  "issuer": "https://issuer.example.com/realms/openshell",
  "client_id": "openshell-cli"
}`
	if string(got) != want {
		t.Errorf("Marshal mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestDiskBundle_MarshalOmitsNilOptionals(t *testing.T) {
	b := DiskBundle{AccessToken: "abc", Issuer: "https://i", ClientID: "c"}
	got, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "access_token": "abc",
  "issuer": "https://i",
  "client_id": "c"
}`
	if string(got) != want {
		t.Errorf("Marshal mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestDiskBundleSource_ValidToken(t *testing.T) {
	now := time.Unix(1000, 0)
	jwt := makeJWT(t, map[string]any{"iss": "https://i", "sub": "user-1", "aud": "openshell-cli"})
	bundle := `{"access_token":"` + jwt + `","expires_at":5000,"issuer":"https://i","client_id":"openshell-cli"}`
	fsys := fstest.MapFS{"oidc_token.json": {Data: []byte(bundle)}}

	src := NewDiskBundleSource(fsys, func() time.Time { return now })
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.Source != SourceDisk {
		t.Errorf("Source = %v", tok.Source)
	}
	if tok.Subject != "user-1" {
		t.Errorf("Subject = %q", tok.Subject)
	}
	if !tok.Expiry.Equal(time.Unix(5000, 0)) {
		t.Errorf("Expiry = %v, want 5000", tok.Expiry)
	}
}

func TestDiskBundleSource_ExpiryBoundary(t *testing.T) {
	// expires_at=1030, leeway 30 in is_token_expired: expired when now+30>=1030,
	// i.e. now>=1000.
	bundleAt := func(exp int64) fstest.MapFS {
		return fstest.MapFS{"oidc_token.json": {Data: []byte(
			`{"access_token":"x","expires_at":` + strconv.FormatInt(exp, 10) + `,"issuer":"https://i","client_id":"c"}`)}}
	}

	// now=999 -> not expired (999+30=1029 < 1030)
	src := NewDiskBundleSource(bundleAt(1030), func() time.Time { return time.Unix(999, 0) })
	if _, err := src.Token(context.Background()); err != nil {
		t.Errorf("now=999 should be valid, got %v", err)
	}
	// now=1000 -> expired (1000+30=1030 >= 1030), no refresh -> ErrTokenExpired
	src2 := NewDiskBundleSource(bundleAt(1030), func() time.Time { return time.Unix(1000, 0) })
	_, err := src2.Token(context.Background())
	var expired *ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Errorf("now=1000 should be expired, got %v", err)
	}
}

func TestDiskBundleSource_ExpiredHintNoRefreshToken(t *testing.T) {
	bundle := fstest.MapFS{"oidc_token.json": {Data: []byte(
		`{"access_token":"x","expires_at":100,"issuer":"https://i","client_id":"c"}`)}}
	src := NewDiskBundleSource(bundle, func() time.Time { return time.Unix(1000, 0) },
		WithBundleGatewayName("my-gw"))
	_, err := src.Token(context.Background())
	var expired *ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
	if !strings.Contains(expired.Hint, "openshellctl login") {
		t.Errorf("hint should suggest openshellctl login, got: %s", expired.Hint)
	}
	if strings.Contains(expired.Hint, "token refresh") {
		t.Errorf("hint should not suggest token refresh without a refresh token, got: %s", expired.Hint)
	}
}

func TestDiskBundleSource_ExpiredHintWithRefreshToken(t *testing.T) {
	r := "refresh-tok"
	bundle := fstest.MapFS{"oidc_token.json": {Data: []byte(
		`{"access_token":"x","refresh_token":"` + r + `","expires_at":100,"issuer":"https://i","client_id":"c"}`)}}
	// No refresher wired, so refresh will fail and we get ErrTokenExpired... wait,
	// actually without a refresher it won't attempt refresh. Let me check the code path.
	// Line 146: if bundle.RefreshToken != nil && *bundle.RefreshToken != "" && s.refresher != nil
	// Without refresher, it falls through to the hint. But the hint checks bundle.RefreshToken.
	src := NewDiskBundleSource(bundle, func() time.Time { return time.Unix(1000, 0) },
		WithBundleGatewayName("my-gw"))
	_, err := src.Token(context.Background())
	var expired *ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
	if !strings.Contains(expired.Hint, "openshellctl token refresh --write") {
		t.Errorf("hint should suggest token refresh when refresh token exists, got: %s", expired.Hint)
	}
}

func TestDiskBundleSource_MissingExpiresAtUsesJWT(t *testing.T) {
	now := time.Unix(1000, 0)
	jwt := makeJWT(t, map[string]any{"iss": "https://i", "exp": float64(9000)})
	bundle := `{"access_token":"` + jwt + `","issuer":"https://i","client_id":"c"}`
	src := NewDiskBundleSource(fstest.MapFS{"oidc_token.json": {Data: []byte(bundle)}}, func() time.Time { return now })
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if !tok.Expiry.Equal(time.Unix(9000, 0)) {
		t.Errorf("Expiry = %v, want JWT exp 9000", tok.Expiry)
	}
}

// fakeRefresher records a call and returns a canned token.
type fakeRefresher struct {
	called      bool
	newRefresh  *string
	newExpiry   time.Time
	gotRefresh  string
	returnedTok string
}

func (f *fakeRefresher) Refresh(_ context.Context, _, _, refreshToken string) (string, *string, time.Time, error) {
	f.called = true
	f.gotRefresh = refreshToken
	return f.returnedTok, f.newRefresh, f.newExpiry, nil
}

func TestDiskBundleSource_RefreshWriteBackPreservesRefreshToken(t *testing.T) {
	now := time.Unix(2000, 0)
	oldRefresh := "old-refresh"
	bundle := `{"access_token":"stale","refresh_token":"old-refresh","expires_at":100,"issuer":"https://i","client_id":"c"}`
	fsys := fstest.MapFS{"oidc_token.json": {Data: []byte(bundle)}}

	w := &captureWriter{}
	ref := &fakeRefresher{
		returnedTok: "fresh",
		newRefresh:  nil, // no new refresh token returned -> keep old
		newExpiry:   time.Unix(9999, 0),
	}
	src := NewDiskBundleSource(fsys, func() time.Time { return now }, WithRefresher(ref, w))

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if !ref.called {
		t.Fatal("refresher was not called")
	}
	if ref.gotRefresh != oldRefresh {
		t.Errorf("refresher got %q, want %q", ref.gotRefresh, oldRefresh)
	}
	if tok.AccessToken != "fresh" {
		t.Errorf("token = %q, want fresh", tok.AccessToken)
	}
	// Write-back must preserve the old refresh token.
	written, _ := ParseDiskBundle(w.data)
	if written.RefreshToken == nil || *written.RefreshToken != oldRefresh {
		t.Errorf("written refresh_token = %v, want %q preserved", written.RefreshToken, oldRefresh)
	}
	if written.AccessToken != "fresh" {
		t.Errorf("written access_token = %q", written.AccessToken)
	}
}

type captureWriter struct{ data []byte }

func (w *captureWriter) WriteFile(_ string, data []byte, _ fs.FileMode) error {
	w.data = append([]byte(nil), data...)
	return nil
}
