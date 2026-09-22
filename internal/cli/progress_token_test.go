package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

func TestPlainSink(t *testing.T) {
	var b bytes.Buffer
	s := newPlainSink(&b)
	s.Header("demo")
	s.StepActive("Provisioning", "pulling image")
	s.StepActive("Waiting", "")
	s.StepDone("Ready", 3*time.Second)
	s.Warning("heads up")
	s.Error("boom")

	out := b.String()
	for _, want := range []string{
		"Created sandbox: demo",
		"  … Provisioning pulling image",
		"  … Waiting",
		"  ✓ Ready (3s)",
		"  ! heads up",
		"  ✗ boom",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plain sink missing %q in:\n%s", want, out)
		}
	}
}

func TestProvisionTimeoutDefaultAndOverride(t *testing.T) {
	t.Setenv("OPENSHELL_PROVISION_TIMEOUT", "")
	if got := provisionTimeout(); got != 300*time.Second {
		t.Errorf("default = %v, want 300s", got)
	}
	t.Setenv("OPENSHELL_PROVISION_TIMEOUT", "45")
	if got := provisionTimeout(); got != 45*time.Second {
		t.Errorf("override = %v, want 45s", got)
	}
}

func TestTokExpiryStr(t *testing.T) {
	if got := tokExpiryStr(&auth.Token{}); got != "unknown" {
		t.Errorf("zero expiry = %q, want unknown", got)
	}
	exp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := tokExpiryStr(&auth.Token{Expiry: exp}); got != "2026-01-02T03:04:05Z" {
		t.Errorf("expiry = %q", got)
	}
}
