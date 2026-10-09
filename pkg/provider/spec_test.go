package provider

import (
	"maps"
	"testing"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

func TestBuildSpec(t *testing.T) {
	creds := map[string]string{"TOKEN": "abc"}
	cfg := map[string]string{"org": "acme"}
	expiry := map[string]time.Time{"TOKEN": time.Unix(100, 0)}

	got := BuildSpec(creds, cfg, expiry)

	if got.Credentials["TOKEN"] != "abc" {
		t.Errorf("Credentials = %+v", got.Credentials)
	}
	if got.Config["org"] != "acme" {
		t.Errorf("Config = %+v", got.Config)
	}
	if !got.CredentialExpiresAt["TOKEN"].Equal(time.Unix(100, 0)) {
		t.Errorf("CredentialExpiresAt = %+v", got.CredentialExpiresAt)
	}
}

func TestBuildSpec_ZeroExpiryOmitted(t *testing.T) {
	// A zero timestamp means "no expiry set" at create time — nothing to
	// clear yet, so it must not appear in the built spec at all.
	got := BuildSpec(nil, nil, map[string]time.Time{"TOKEN": {}})
	if _, ok := got.CredentialExpiresAt["TOKEN"]; ok {
		t.Errorf("CredentialExpiresAt should omit zero-valued entries, got %+v", got.CredentialExpiresAt)
	}
}

func TestMergeSpec_OverlayAddsAndOverwrites(t *testing.T) {
	existing := types.ProviderSpec{
		Credentials: map[string]string{"TOKEN": "old", "OTHER": "keep-me"},
		Config:      map[string]string{"org": "old-org"},
	}
	overlay := types.ProviderSpec{
		Credentials: map[string]string{"TOKEN": "new"},
		Config:      map[string]string{"org": "new-org", "region": "us"},
	}

	got := MergeSpec(existing, overlay)

	want := map[string]string{"TOKEN": "new", "OTHER": "keep-me"}
	if !maps.Equal(got.Credentials, want) {
		t.Errorf("Credentials = %+v, want %+v", got.Credentials, want)
	}
	wantCfg := map[string]string{"org": "new-org", "region": "us"}
	if !maps.Equal(got.Config, wantCfg) {
		t.Errorf("Config = %+v, want %+v", got.Config, wantCfg)
	}
}

func TestMergeSpec_ZeroExpiryClearsExistingEntry(t *testing.T) {
	existing := types.ProviderSpec{
		CredentialExpiresAt: map[string]time.Time{"TOKEN": time.Unix(100, 0), "OTHER": time.Unix(200, 0)},
	}
	overlay := types.ProviderSpec{
		CredentialExpiresAt: map[string]time.Time{"TOKEN": {}},
	}

	got := MergeSpec(existing, overlay)

	if _, ok := got.CredentialExpiresAt["TOKEN"]; ok {
		t.Errorf("TOKEN expiry should be cleared, got %+v", got.CredentialExpiresAt)
	}
	if !got.CredentialExpiresAt["OTHER"].Equal(time.Unix(200, 0)) {
		t.Errorf("OTHER expiry should be untouched, got %+v", got.CredentialExpiresAt)
	}
}

func TestMergeSpec_EmptyOverlayKeepsExisting(t *testing.T) {
	existing := types.ProviderSpec{
		Credentials: map[string]string{"TOKEN": "old"},
		Config:      map[string]string{"org": "old-org"},
	}
	got := MergeSpec(existing, types.ProviderSpec{})
	if !maps.Equal(got.Credentials, existing.Credentials) {
		t.Errorf("Credentials changed: %+v", got.Credentials)
	}
	if !maps.Equal(got.Config, existing.Config) {
		t.Errorf("Config changed: %+v", got.Config)
	}
}
