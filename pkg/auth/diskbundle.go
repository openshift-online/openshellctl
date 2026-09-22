package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"time"
)

// DiskBundle is the Rust CLI's oidc_token.json (oidc_token.rs:16-35). Field
// order matches the Rust struct so Marshal reproduces serde_json::to_string_pretty
// output byte-for-byte. refresh_token/expires_at are omitted when nil.
type DiskBundle struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken *string `json:"refresh_token,omitempty"`
	ExpiresAt    *int64  `json:"expires_at,omitempty"` // unix seconds; Rust Option<u64>
	Issuer       string  `json:"issuer"`
	ClientID     string  `json:"client_id"`
}

// ParseDiskBundle strictly decodes an oidc_token.json. issuer and client_id are
// required — upstream load_oidc_token silently drops a bundle missing them (they
// are non-Option fields); we surface ErrBundleInvalid so the user learns why.
func ParseDiskBundle(b []byte) (*DiskBundle, error) {
	var db DiskBundle
	if err := json.Unmarshal(b, &db); err != nil {
		return nil, &ErrBundleInvalid{Reason: "not valid JSON: " + err.Error()}
	}
	if db.AccessToken == "" {
		return nil, &ErrBundleInvalid{Reason: "access_token is empty"}
	}
	if db.Issuer == "" {
		return nil, &ErrBundleInvalid{Reason: "issuer is required"}
	}
	if db.ClientID == "" {
		return nil, &ErrBundleInvalid{Reason: "client_id is required"}
	}
	return &db, nil
}

// Marshal renders the bundle as 2-space-indented JSON with keys in struct order,
// matching serde_json::to_string_pretty (no trailing newline). HTML escaping is
// disabled so characters like & in URLs are emitted literally, as serde does.
func (b DiskBundle) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return nil, err
	}
	// json.Encoder appends a trailing newline; serde_json::to_string_pretty does
	// not. Strip it to match byte-for-byte.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}

// isExpired mirrors is_token_expired (oidc_token.rs:115-125): expired when
// now + 30s >= expires_at. A missing expires_at is treated as NOT expired.
func (b DiskBundle) isExpired(now time.Time) bool {
	if b.ExpiresAt == nil {
		return false
	}
	return now.Unix()+30 >= *b.ExpiresAt
}

// RefreshTokenExchanger runs an OAuth2 refresh-token grant. It is the network
// boundary for DiskBundleSource refresh; production wires it to x/oauth2 with a
// discovered token endpoint (spec §5.2).
type RefreshTokenExchanger interface {
	// Refresh exchanges a refresh token, returning the new access token, the
	// (possibly rotated) refresh token, and the new expiry. A nil returned
	// refresh token means "keep the old one".
	Refresh(ctx context.Context, issuer, clientID, refreshToken string) (accessToken string, newRefresh *string, expiry time.Time, err error)
}

// DiskBundleSource reads a Rust-schema oidc_token.json on every Token() call
// (so an external rotator is honoured) and, when configured with a refresher,
// runs the refresh-token grant on expiry.
type DiskBundleSource struct {
	fsys      fs.FS
	relPath   string
	clock     func() time.Time
	refresher RefreshTokenExchanger // nil ok
	writer    Writer                // nil ok (needed to persist a refresh)
	gateway   string                // for Describe / write-back path
}

// Writer abstracts the write-back of a refreshed bundle. It mirrors
// gatewayconfig.Writer to avoid an import cycle; the CLI passes a small adapter.
type Writer interface {
	WriteFile(relPath string, data []byte, perm fs.FileMode) error
}

// DiskBundleOption configures a DiskBundleSource.
type DiskBundleOption func(*DiskBundleSource)

// WithRefresher sets the refresh-token exchanger and the writer used to persist
// a refreshed bundle.
func WithRefresher(r RefreshTokenExchanger, w Writer) DiskBundleOption {
	return func(s *DiskBundleSource) { s.refresher = r; s.writer = w }
}

// WithBundleGatewayName sets the gateway name for Describe and the write path.
func WithBundleGatewayName(name string) DiskBundleOption {
	return func(s *DiskBundleSource) { s.gateway = name }
}

// NewDiskBundleSource builds a source reading relPath (default "oidc_token.json")
// from fsys. clock defaults to time.Now.
func NewDiskBundleSource(fsys fs.FS, clock func() time.Time, opts ...DiskBundleOption) *DiskBundleSource {
	if clock == nil {
		clock = time.Now
	}
	s := &DiskBundleSource{fsys: fsys, relPath: "oidc_token.json", clock: clock}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Token reads the bundle, maps expires_at → Expiry, and fills the rest from the
// JWT. On expiry: if a refresh token and refresher are present, it runs the
// grant, writes back (preserving the old refresh token when none is returned),
// and returns the new token; otherwise it returns *ErrTokenExpired with a hint.
func (s *DiskBundleSource) Token(ctx context.Context) (*Token, error) {
	b, err := fs.ReadFile(s.fsys, s.relPath)
	if err != nil {
		return nil, &ErrBundleInvalid{Reason: "cannot read " + s.relPath + ": " + err.Error()}
	}
	bundle, err := ParseDiskBundle(b)
	if err != nil {
		return nil, err
	}
	now := s.clock()

	if !bundle.isExpired(now) {
		return s.tokenFromBundle(bundle, now), nil
	}

	// Expired. Try refresh if possible.
	if bundle.RefreshToken != nil && *bundle.RefreshToken != "" && s.refresher != nil {
		return s.refresh(ctx, bundle, now)
	}

	tok := s.tokenFromBundle(bundle, now)
	hint := "re-login with `openshellctl login -g " + s.gateway + "`, or set OPENSHELL_OIDC_CLIENT_SECRET for automatic renewal"
	if bundle.RefreshToken != nil && *bundle.RefreshToken != "" {
		hint = "run `openshellctl token refresh --write` to obtain a new token, or set OPENSHELL_OIDC_CLIENT_SECRET for automatic renewal"
	}
	return nil, &ErrTokenExpired{
		Expiry: tok.Expiry,
		Age:    tok.Age(now),
		Hint:   hint,
	}
}

func (s *DiskBundleSource) refresh(ctx context.Context, bundle *DiskBundle, now time.Time) (*Token, error) {
	access, newRefresh, expiry, err := s.refresher.Refresh(ctx, bundle.Issuer, bundle.ClientID, *bundle.RefreshToken)
	if err != nil {
		return nil, &ExchangeError{Cause: err}
	}
	refreshToken := bundle.RefreshToken
	if newRefresh != nil {
		refreshToken = newRefresh
	}
	newBundle := &DiskBundle{
		AccessToken:  access,
		RefreshToken: refreshToken,
		Issuer:       bundle.Issuer,
		ClientID:     bundle.ClientID,
	}
	if !expiry.IsZero() {
		exp := expiry.Unix()
		newBundle.ExpiresAt = &exp
	}
	if s.writer != nil {
		if data, merr := newBundle.Marshal(); merr == nil {
			_ = s.writer.WriteFile(s.relPath, data, 0o600)
		}
	}
	return s.tokenFromBundle(newBundle, now), nil
}

// tokenFromBundle builds a Token from a bundle, using expires_at for Expiry and
// the JWT for iat/sub/aud/roles.
func (s *DiskBundleSource) tokenFromBundle(b *DiskBundle, now time.Time) *Token {
	tok := &Token{
		AccessToken: b.AccessToken,
		Issuer:      b.Issuer,
		ClientID:    b.ClientID,
		Source:      SourceDisk,
		IssuedAt:    now,
	}
	if b.ExpiresAt != nil {
		tok.Expiry = time.Unix(*b.ExpiresAt, 0)
	}
	if c, err := Inspect(b.AccessToken); err == nil {
		tok.Subject = c.Sub
		tok.Audience = c.Aud
		tok.Roles = c.Roles
		if b.ExpiresAt == nil && c.Exp > 0 {
			tok.Expiry = time.Unix(c.Exp, 0) // fall back to JWT exp
		}
		if c.Iat > 0 {
			tok.IssuedAt = time.Unix(c.Iat, 0)
		}
		if tok.Issuer == "" {
			tok.Issuer = c.Iss
		}
	}
	return tok
}

// Invalidate is a no-op: the bundle is re-read on every Token() call.
func (s *DiskBundleSource) Invalidate() {}

// Describe implements TokenSource.
func (s *DiskBundleSource) Describe() string {
	if s.gateway != "" {
		return "oidc_token.json (gateway=" + s.gateway + ")"
	}
	return "oidc_token.json"
}
