package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestErrorMessages(t *testing.T) {
	expired := &ErrTokenExpired{Expiry: time.Unix(0, 0), Hint: "do X"}
	if got := expired.Error(); got == "" || !contains(got, "do X") {
		t.Errorf("ErrTokenExpired message = %q", got)
	}
	expiredNoHint := &ErrTokenExpired{Expiry: time.Unix(0, 0)}
	if contains(expiredNoHint.Error(), ";") {
		t.Errorf("no-hint message should not have a hint separator: %q", expiredNoHint.Error())
	}

	bundle := &ErrBundleInvalid{Reason: "issuer is required"}
	if !contains(bundle.Error(), "issuer is required") {
		t.Errorf("ErrBundleInvalid message = %q", bundle.Error())
	}

	xe := &ExchangeError{Cause: errors.New("boom")}
	if !contains(xe.Error(), "boom") {
		t.Errorf("ExchangeError message = %q", xe.Error())
	}
	if !errors.Is(xe, xe) || xe.Unwrap() == nil {
		t.Error("ExchangeError should unwrap its cause")
	}

	missing := &ErrOIDCConfigMissing{Checked: []string{"a", "b"}}
	if !contains(missing.Error(), "a") || !contains(missing.Error(), "b") {
		t.Errorf("ErrOIDCConfigMissing message = %q", missing.Error())
	}
	if contains((&ErrOIDCConfigMissing{}).Error(), "checked") {
		t.Errorf("empty checked list should omit the parenthetical")
	}

	mtls := &ErrMTLSMaterialMissing{Gateway: "rosa"}
	if !contains(mtls.Error(), "rosa") {
		t.Errorf("ErrMTLSMaterialMissing message = %q", mtls.Error())
	}

	unsupported := &ErrUnsupportedAuthMode{Mode: "cloudflare_jwt"}
	if !contains(unsupported.Error(), "cloudflare_jwt") {
		t.Errorf("ErrUnsupportedAuthMode message = %q", unsupported.Error())
	}
}
