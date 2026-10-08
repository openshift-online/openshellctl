package doctor

import (
	"fmt"
	"slices"
	"strings"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/sandbox"
)

// gatewayRoles mirrors auth.PreflightWarnings' role check (the gateway
// accepts either; see pkg/auth/preflight.go) — duplicated here, not
// imported, because CheckRoles needs its own independent pass/fail/NextStep
// shape rather than PreflightWarnings' combined-string-slice return.
var gatewayRoles = []string{"openshell-user", "openshell-admin"}

// CheckEndpointURL confirms the resolved gateway endpoint normalizes
// cleanly (gatewayconfig.NormalizeEndpoint, Feature C) — a malformed or
// empty endpoint means every later check is meaningless.
func CheckEndpointURL(endpoint string) CheckResult {
	if endpoint == "" {
		return CheckResult{
			Name:   "Endpoint URL",
			Status: StatusFail,
			Detail: "no gateway endpoint configured",
			NextStep: "set --gateway-endpoint / OPENSHELL_GATEWAY_ENDPOINT, or register one with " +
				"`openshellctl gateway add <endpoint> --name <name>`",
		}
	}
	norm, err := gatewayconfig.NormalizeEndpoint(endpoint)
	if err != nil {
		return CheckResult{
			Name:     "Endpoint URL",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("endpoint %q is not a valid URL: %v", endpoint, err),
			NextStep: "fix --gateway-endpoint / OPENSHELL_GATEWAY_ENDPOINT / the registered gateway's metadata.json",
		}
	}
	return CheckResult{
		Name:   "Endpoint URL",
		Status: StatusPass,
		Detail: fmt.Sprintf("%s (normalized: %s)", endpoint, norm),
	}
}

// CheckAudience confirms the minted token's audience includes wantAudience.
// An empty wantAudience (no --oidc-audience and no metadata.oidc_audience —
// see internal/cli/token.go's expectedAudience, which always resolves to at
// least the "openshell-cli" default, so in practice this is only empty when
// the caller has nothing at all to compare against) skips rather than
// passing or failing — there is nothing to check.
func CheckAudience(tok *auth.Token, wantAudience string) CheckResult {
	if wantAudience == "" {
		return CheckResult{Name: "Audience", Status: StatusSkip, Detail: "no expected audience to check"}
	}
	if tok == nil {
		return CheckResult{
			Name: "Audience", Status: StatusFail, Detail: "no token to check",
			NextStep: "resolve the Credentials check's failure first",
		}
	}
	if slices.Contains(tok.Audience, wantAudience) {
		return CheckResult{
			Name: "Audience", Status: StatusPass,
			Detail: fmt.Sprintf("token audience %v includes %q", tok.Audience, wantAudience),
		}
	}
	return CheckResult{
		Name:   "Audience",
		Status: StatusFail,
		Detail: fmt.Sprintf("token audience %v does not include the expected audience %q", tok.Audience, wantAudience),
		NextStep: "check --oidc-audience / metadata.oidc_audience against the gateway's /auth/oidc-config, " +
			"then `openshellctl token refresh`",
	}
}

// CheckRoles confirms the minted token carries a role the gateway actually
// accepts (openshell-user or openshell-admin — authz.rs:85-101 at the pinned
// upstream commit). Mirrors auth.PreflightWarnings' role rule: "has any role
// at all" is not enough, since a default Keycloak human token already
// carries default-roles-<realm>/offline_access.
func CheckRoles(tok *auth.Token) CheckResult {
	if tok == nil {
		return CheckResult{
			Name: "Roles", Status: StatusFail, Detail: "no token to check",
			NextStep: "resolve the Credentials check's failure first",
		}
	}
	if slices.ContainsFunc(gatewayRoles, func(r string) bool { return slices.Contains(tok.Roles, r) }) {
		return CheckResult{Name: "Roles", Status: StatusPass, Detail: fmt.Sprintf("token roles %v", tok.Roles)}
	}
	return CheckResult{
		Name:   "Roles",
		Status: StatusFail,
		Detail: fmt.Sprintf("token roles %v include neither %q nor %q", tok.Roles, gatewayRoles[0], gatewayRoles[1]),
		NextStep: "ask an administrator to grant the \"openshell-user\" realm role (\"openshell-admin\" also " +
			"satisfies it) to this account, or use a service account client that already has it",
	}
}

// CheckExpiry confirms the minted token has a known, future expiry. A zero
// Expiry means the gateway's token response omitted expires_in (the
// auth.ErrNoExpiry concern, pkg/auth/errors.go) — this is flagged as a
// failure here (not merely unknown) because every downstream caller assumes
// Expiry is meaningful.
func CheckExpiry(tok *auth.Token, now time.Time) CheckResult {
	if tok == nil {
		return CheckResult{
			Name: "Expiry", Status: StatusFail, Detail: "no token to check",
			NextStep: "resolve the Credentials check's failure first",
		}
	}
	if tok.Expiry.IsZero() {
		return CheckResult{
			Name:     "Expiry",
			Status:   StatusFail,
			Detail:   "token has no expiry (expires_in was not returned by the token exchange)",
			NextStep: "contact the gateway administrator — the token endpoint should return expires_in",
		}
	}
	if !tok.Expiry.After(now) {
		return CheckResult{
			Name:     "Expiry",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("token expired at %s", tok.Expiry.UTC().Format(time.RFC3339)),
			NextStep: "run `openshellctl token refresh`",
		}
	}
	return CheckResult{
		Name:   "Expiry",
		Status: StatusPass,
		Detail: fmt.Sprintf("token expires at %s (in %s)", tok.Expiry.UTC().Format(time.RFC3339), tok.Expiry.Sub(now).Round(time.Second)),
	}
}

// CheckOIDCConfigMatch flags metadata drift: the registered gateway's
// configured issuer/audience no longer matching what the gateway's own
// /auth/oidc-config actually serves. Skips when there's nothing on either
// side to compare (no discovery result — the HTTP check didn't run or
// failed; or no registered gateway at all — an endpoint-only invocation). A
// registered value of "" is treated as "no claim made" for that field and
// is not compared (metadata.oidc_audience is optional; see
// Metadata.OIDCAudienceOrDefault for how a caller should normally resolve it
// before calling this, but an explicit "" here still degrades gracefully).
func CheckOIDCConfigMatch(registeredIssuer, registeredAudience, discoveredIssuer, discoveredAudience string) CheckResult {
	if discoveredIssuer == "" && discoveredAudience == "" {
		return CheckResult{Name: "OIDC config match", Status: StatusSkip, Detail: "no discovered OIDC config to compare"}
	}
	if registeredIssuer == "" && registeredAudience == "" {
		return CheckResult{Name: "OIDC config match", Status: StatusSkip, Detail: "no registered gateway metadata to compare"}
	}
	var mismatches []string
	if registeredIssuer != "" && registeredIssuer != discoveredIssuer {
		mismatches = append(mismatches, fmt.Sprintf("issuer: registered %q, discovered %q", registeredIssuer, discoveredIssuer))
	}
	if registeredAudience != "" && registeredAudience != discoveredAudience {
		mismatches = append(mismatches, fmt.Sprintf("audience: registered %q, discovered %q", registeredAudience, discoveredAudience))
	}
	if len(mismatches) > 0 {
		return CheckResult{
			Name:   "OIDC config match",
			Status: StatusFail,
			Detail: "registered metadata does not match the gateway's /auth/oidc-config: " + strings.Join(mismatches, "; "),
			NextStep: "re-register with `openshellctl gateway add <endpoint> --name <name> --force` to pick up " +
				"the gateway's current OIDC config",
		}
	}
	return CheckResult{Name: "OIDC config match", Status: StatusPass, Detail: "registered metadata matches /auth/oidc-config"}
}

// CheckProviders confirms every requested provider (by name or recognized
// type) exists on the gateway, reusing sandbox.ResolveProviders — the same
// logic `sandbox create --provider` uses — rather than reimplementing name/
// type resolution. Skips entirely when nothing was requested (no
// --provider/-f given to `doctor`): providers are opt-in to check, not a
// baseline preflight requirement.
func CheckProviders(known []*types.Provider, requested []string) CheckResult {
	if len(requested) == 0 {
		return CheckResult{Name: "Providers", Status: StatusSkip, Detail: "no --provider/-f given"}
	}
	res, err := sandbox.ResolveProviders(known, requested, nil)
	if err != nil {
		return CheckResult{
			Name:     "Providers",
			Status:   StatusFail,
			Detail:   err.Error(),
			NextStep: "openshellctl sandbox provider create --type <type> --name <name>",
		}
	}
	if len(res.MissingTypes) > 0 {
		return CheckResult{
			Name:     "Providers",
			Status:   StatusFail,
			Detail:   fmt.Sprintf("provider type(s) %v recognized but not yet created on this gateway", res.MissingTypes),
			NextStep: fmt.Sprintf("openshellctl sandbox provider create --type %s --name <name>", res.MissingTypes[0]),
		}
	}
	return CheckResult{
		Name:   "Providers",
		Status: StatusPass,
		Detail: fmt.Sprintf("all requested providers exist: %v", res.Names),
	}
}
