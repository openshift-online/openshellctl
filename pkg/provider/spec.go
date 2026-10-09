package provider

import (
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

// BuildSpec builds a fresh ProviderSpec from parsed flag values, for
// `provider create`. A zero-valued expiry (ParseCredentialExpiresAt's
// "clear" sentinel) is meaningless on a brand-new provider, so it is
// omitted rather than stored.
func BuildSpec(credentials, config map[string]string, expiresAt map[string]time.Time) types.ProviderSpec {
	spec := types.ProviderSpec{
		Credentials: credentials,
		Config:      config,
	}
	for key, t := range expiresAt {
		if t.IsZero() {
			continue
		}
		if spec.CredentialExpiresAt == nil {
			spec.CredentialExpiresAt = map[string]time.Time{}
		}
		spec.CredentialExpiresAt[key] = t
	}
	return spec
}

// MergeSpec applies overlay onto existing, for `provider update`: each
// overlay map key is set on (or added to) the corresponding existing map,
// and keys existing has but overlay doesn't are left untouched. A zero-valued
// entry in overlay.CredentialExpiresAt clears that key from the result
// instead of setting it.
func MergeSpec(existing, overlay types.ProviderSpec) types.ProviderSpec {
	merged := types.ProviderSpec{
		Credentials:         mergeStringMap(existing.Credentials, overlay.Credentials),
		Config:              mergeStringMap(existing.Config, overlay.Config),
		CredentialExpiresAt: mergeExpiryMap(existing.CredentialExpiresAt, overlay.CredentialExpiresAt),
		ProfileWorkspace:    existing.ProfileWorkspace,
		CredentialHandles:   existing.CredentialHandles,
	}
	return merged
}

func mergeStringMap(existing, overlay map[string]string) map[string]string {
	if len(existing) == 0 && len(overlay) == 0 {
		return existing
	}
	merged := make(map[string]string, len(existing)+len(overlay))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

func mergeExpiryMap(existing, overlay map[string]time.Time) map[string]time.Time {
	if len(existing) == 0 && len(overlay) == 0 {
		return existing
	}
	merged := make(map[string]time.Time, len(existing)+len(overlay))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range overlay {
		if v.IsZero() {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	return merged
}
