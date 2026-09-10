package auth

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Exchanger is the one network boundary for client-credentials auth. The
// production implementation wraps the SDK's oidc.ClientCredentials.
type Exchanger interface {
	Exchange(ctx context.Context, cfg ClientCredentialsConfig) (accessToken string, expiresIn time.Duration, err error)
}

// ClientCredentialsConfig is the resolved OIDC client-credentials input.
type ClientCredentialsConfig struct {
	Issuer   string
	ClientID string
	Audience string
	Scopes   []string
	Secret   func(ctx context.Context) (string, error)
	Timeout  time.Duration // default 30s
}

// ClientCredentialsSource caches a client-credentials token, re-exchanging under
// a singleflight group when the cached token is within leeway of expiry. On
// exchange failure it returns a still-valid cached token (logging a warning) or,
// if the cache is past exp, an *ExchangeError after applying backoff.
type ClientCredentialsSource struct {
	cfg     ClientCredentialsConfig
	ex      Exchanger
	clock   func() time.Time
	leeway  time.Duration
	group   singleflight.Group
	gateway string

	backoffInitial time.Duration
	backoffMax     time.Duration

	mu         sync.Mutex
	cached     *Token
	nextTry    time.Time // earliest time a retry is allowed after a failure
	curBackoff time.Duration
}

// CCOption configures a ClientCredentialsSource.
type CCOption func(*ClientCredentialsSource)

// WithLeeway sets the re-exchange leeway (default 30s, matching the SDK).
func WithLeeway(d time.Duration) CCOption {
	return func(s *ClientCredentialsSource) { s.leeway = d }
}

// WithClock injects a clock (default time.Now).
func WithClock(f func() time.Time) CCOption {
	return func(s *ClientCredentialsSource) { s.clock = f }
}

// WithBackoff sets the failure backoff bounds (default 1s → 30s, doubling).
func WithBackoff(initial, max time.Duration) CCOption {
	return func(s *ClientCredentialsSource) { s.backoffInitial = initial; s.backoffMax = max }
}

// WithGatewayName sets the gateway name for Describe.
func WithGatewayName(name string) CCOption {
	return func(s *ClientCredentialsSource) { s.gateway = name }
}

// NewClientCredentialsSource builds a client-credentials token source.
func NewClientCredentialsSource(cfg ClientCredentialsConfig, ex Exchanger, opts ...CCOption) *ClientCredentialsSource {
	s := &ClientCredentialsSource{
		cfg:            cfg,
		ex:             ex,
		clock:          time.Now,
		leeway:         30 * time.Second,
		backoffInitial: time.Second,
		backoffMax:     30 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}
	if s.cfg.Timeout == 0 {
		s.cfg.Timeout = 30 * time.Second
	}
	return s
}

// Token returns a cached token when it is still outside the leeway window;
// otherwise it exchanges (coalesced via singleflight). See the type doc for
// failure handling.
func (s *ClientCredentialsSource) Token(ctx context.Context) (*Token, error) {
	now := s.clock()

	s.mu.Lock()
	cached := s.cached
	s.mu.Unlock()

	if cached != nil && !cached.Expired(now, s.leeway) {
		return cached, nil
	}

	v, err, _ := s.group.Do("exchange", func() (any, error) {
		return s.exchange(ctx)
	})
	if err != nil {
		return nil, err
	}
	return v.(*Token), nil
}

// exchange performs (or coalesces) a single exchange, applying the failure
// policy. It is only ever invoked inside the singleflight group.
func (s *ClientCredentialsSource) exchange(ctx context.Context) (*Token, error) {
	now := s.clock()

	// Re-check the cache: another caller may have refreshed while we queued.
	s.mu.Lock()
	if s.cached != nil && !s.cached.Expired(now, s.leeway) {
		tok := s.cached
		s.mu.Unlock()
		return tok, nil
	}
	// Respect backoff after a recent failure, but only when we have no usable
	// cache to fall back on.
	if !s.nextTry.IsZero() && now.Before(s.nextTry) && (s.cached == nil || s.cached.Expired(now, 0)) {
		s.mu.Unlock()
		return nil, &ExchangeError{Cause: errBackoff}
	}
	cached := s.cached
	s.mu.Unlock()

	// The Exchanger resolves the client secret itself via cfg.Secret (the
	// production impl wires it through WithClientSecretProvider), so a secret
	// read failure surfaces as an exchange error here.
	access, expiresIn, err := s.ex.Exchange(ctx, s.cfg)
	if err != nil {
		return s.onFailure(now, cached, err)
	}
	if expiresIn <= 0 {
		return nil, ErrNoExpiry
	}

	tok := &Token{
		AccessToken: access,
		Source:      SourceClientCredentials,
		Issuer:      s.cfg.Issuer,
		ClientID:    s.cfg.ClientID,
		IssuedAt:    now,
		Expiry:      now.Add(expiresIn),
	}
	// Cross-check against the JWT's own claims; prefer the earlier expiry.
	if c, ierr := Inspect(access); ierr == nil {
		tok.Subject = c.Sub
		tok.Audience = c.Aud
		tok.Roles = c.Roles
		if c.Iat > 0 {
			tok.IssuedAt = time.Unix(c.Iat, 0)
		}
		if c.Exp > 0 {
			jwtExp := time.Unix(c.Exp, 0)
			if jwtExp.Before(tok.Expiry) {
				tok.Expiry = jwtExp
			}
		}
	}

	s.mu.Lock()
	s.cached = tok
	s.nextTry = time.Time{}
	s.curBackoff = 0
	s.mu.Unlock()
	return tok, nil
}

// onFailure applies the failure policy: return the cached token if it is still
// within its hard expiry; otherwise record backoff and return an ExchangeError.
func (s *ClientCredentialsSource) onFailure(now time.Time, cached *Token, cause error) (*Token, error) {
	if cached != nil && !cached.Expired(now, 0) {
		return cached, nil // still valid within exp; serve stale, caller may warn
	}
	s.mu.Lock()
	if s.curBackoff == 0 {
		s.curBackoff = s.backoffInitial
	} else {
		s.curBackoff *= 2
		if s.curBackoff > s.backoffMax {
			s.curBackoff = s.backoffMax
		}
	}
	s.nextTry = now.Add(s.curBackoff)
	s.mu.Unlock()
	return nil, &ExchangeError{Cause: cause}
}

// Invalidate drops the cached token so the next Token() re-exchanges.
func (s *ClientCredentialsSource) Invalidate() {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
}

// Describe implements TokenSource.
func (s *ClientCredentialsSource) Describe() string {
	if s.gateway != "" {
		return "client_credentials via metadata.json (gateway=" + s.gateway + ")"
	}
	return "client_credentials"
}
