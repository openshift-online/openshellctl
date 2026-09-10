package auth

import (
	"math"
	"testing"
	"time"
)

func TestToken_Age(t *testing.T) {
	now := time.Unix(1000, 0)
	tok := Token{IssuedAt: time.Unix(700, 0)}
	if got := tok.Age(now); got != 300*time.Second {
		t.Errorf("Age = %v, want 300s", got)
	}
}

func TestToken_ExpiresIn(t *testing.T) {
	now := time.Unix(1000, 0)

	future := Token{Expiry: time.Unix(1300, 0)}
	if got := future.ExpiresIn(now); got != 300*time.Second {
		t.Errorf("ExpiresIn(future) = %v, want 300s", got)
	}

	past := Token{Expiry: time.Unix(900, 0)}
	if got := past.ExpiresIn(now); got != -100*time.Second {
		t.Errorf("ExpiresIn(past) = %v, want -100s", got)
	}

	unknown := Token{} // zero Expiry
	if got := unknown.ExpiresIn(now); got != time.Duration(math.MaxInt64) {
		t.Errorf("ExpiresIn(zero) = %v, want MaxInt64", got)
	}
}

func TestToken_Expired(t *testing.T) {
	now := time.Unix(1000, 0)
	leeway := 30 * time.Second

	// Expires at 1020; with 30s leeway it is considered expired at now=1000
	// because 1000 >= 1020-30 (990).
	soon := Token{Expiry: time.Unix(1020, 0)}
	if !soon.Expired(now, leeway) {
		t.Errorf("token expiring within leeway should be Expired")
	}

	// Expires at 1100; 1000 < 1100-30 (1070) => not expired.
	later := Token{Expiry: time.Unix(1100, 0)}
	if later.Expired(now, leeway) {
		t.Errorf("token beyond leeway should not be Expired")
	}

	// Zero Expiry is never expired.
	unknown := Token{}
	if unknown.Expired(now, leeway) {
		t.Errorf("zero-Expiry token should never be Expired")
	}
}
