package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeExchanger is a scripted Exchanger.
type fakeExchanger struct {
	mu        sync.Mutex
	calls     int32
	expiresIn time.Duration
	token     string
	err       error
	delay     time.Duration
}

func (f *fakeExchanger) Exchange(_ context.Context, _ ClientCredentialsConfig) (string, time.Duration, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", 0, f.err
	}
	return f.token, f.expiresIn, nil
}

func (f *fakeExchanger) count() int32 { return atomic.LoadInt32(&f.calls) }

func ccConfig() ClientCredentialsConfig {
	return ClientCredentialsConfig{
		Issuer:   "https://i",
		ClientID: "c",
		Secret:   func(context.Context) (string, error) { return "s", nil },
	}
}

func TestCCSource_FirstCallExchanges(t *testing.T) {
	now := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return now }))

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "tok" || tok.Source != SourceClientCredentials {
		t.Errorf("unexpected token %+v", tok)
	}
	if !tok.Expiry.Equal(now.Add(time.Hour)) {
		t.Errorf("Expiry = %v, want now+1h", tok.Expiry)
	}
	if ex.count() != 1 {
		t.Errorf("exchanges = %d, want 1", ex.count())
	}
}

func TestCCSource_WithinLeewayReuses(t *testing.T) {
	cur := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return cur }))

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Advance a little, still well within (expiry - leeway).
	cur = cur.Add(10 * time.Minute)
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ex.count() != 1 {
		t.Errorf("exchanges = %d, want 1 (cached reuse)", ex.count())
	}
}

func TestCCSource_ReexchangesAtLeewayBoundary(t *testing.T) {
	cur := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex,
		WithClock(func() time.Time { return cur }), WithLeeway(30*time.Second))

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Move to expiry-leeway: 1000+3600-30 = 4570.
	cur = time.Unix(4570, 0)
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ex.count() != 2 {
		t.Errorf("exchanges = %d, want 2 (re-exchange at leeway)", ex.count())
	}
}

func TestCCSource_Singleflight(t *testing.T) {
	now := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: time.Hour, delay: 20 * time.Millisecond}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return now }))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = src.Token(context.Background())
		}()
	}
	wg.Wait()
	if ex.count() != 1 {
		t.Errorf("exchanges = %d, want 1 under singleflight", ex.count())
	}
}

func TestCCSource_FailureWithValidCacheReturnsCache(t *testing.T) {
	cur := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return cur }))

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Enter the leeway window (triggers a re-exchange) but stay before hard exp,
	// then make the exchange fail: the still-valid cache must be served.
	ex.mu.Lock()
	ex.err = errors.New("network down")
	ex.mu.Unlock()
	cur = time.Unix(1000+3600-10, 0) // 10s before exp, inside 30s leeway

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("expected cached token on failure, got err %v", err)
	}
	if tok.AccessToken != "tok" {
		t.Errorf("token = %q, want cached tok", tok.AccessToken)
	}
}

func TestCCSource_FailureWithExpiredCacheErrors(t *testing.T) {
	cur := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return cur }))

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	src.Invalidate()
	ex.mu.Lock()
	ex.err = errors.New("network down")
	ex.mu.Unlock()
	cur = time.Unix(1000+3601, 0) // past exp

	_, err := src.Token(context.Background())
	var xe *ExchangeError
	if !errors.As(err, &xe) {
		t.Fatalf("err = %v, want ExchangeError", err)
	}
}

func TestCCSource_ExpiresInNonPositive(t *testing.T) {
	now := time.Unix(1000, 0)
	ex := &fakeExchanger{token: "tok", expiresIn: 0}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return now }))

	_, err := src.Token(context.Background())
	if !errors.Is(err, ErrNoExpiry) {
		t.Fatalf("err = %v, want ErrNoExpiry", err)
	}
}

func TestCCSource_JWTExpDisagreementPrefersEarlier(t *testing.T) {
	now := time.Unix(1000, 0)
	// expires_in says 1h (exp=4600), but JWT exp says 2000 (earlier). Earlier wins.
	jwt := makeJWT(t, map[string]any{"iss": "https://i", "sub": "u", "exp": float64(2000)})
	ex := &fakeExchanger{token: jwt, expiresIn: time.Hour}
	src := NewClientCredentialsSource(ccConfig(), ex, WithClock(func() time.Time { return now }))

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !tok.Expiry.Equal(time.Unix(2000, 0)) {
		t.Errorf("Expiry = %v, want earlier JWT exp 2000", tok.Expiry)
	}
}

func TestCCSource_Describe(t *testing.T) {
	src := NewClientCredentialsSource(ccConfig(), &fakeExchanger{}, WithGatewayName("rosa"))
	if got := src.Describe(); got == "" {
		t.Error("Describe should be non-empty")
	}
}
