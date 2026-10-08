package auth

import (
	"fmt"
	"slices"
)

// gatewayRoles are the realm_access.roles the gateway actually checks
// (authz.rs:85-101 at the pinned upstream commit): openshell-user is the
// minimum needed for any sandbox/gateway RPC; openshell-admin satisfies the
// same check (it is a superset). Any other roles the token carries (e.g.
// default-roles-<realm>, offline_access — both common on a default Keycloak
// human token) are irrelevant to this check and must not be mistaken for
// one of these two.
var gatewayRoles = []string{"openshell-user", "openshell-admin"}

// PreflightWarnings returns zero or more warnings about tok that a user
// should see before relying on it for gateway calls — e.g. before a
// permission-denied surprise. Pure: no I/O, no network.
//
// wantAudience, when non-empty, is checked against tok.Audience (already
// normalized to []string regardless of whether the original JWT aud claim
// was a string or an array — see jwtinspect.go's audienceClaim). An empty
// wantAudience skips the audience check entirely (the caller has no
// expected audience configured, e.g. no --oidc-audience given).
//
// A tok missing both openshell-user and openshell-admin is warned about,
// since every gateway RPC requires one of the two (see gatewayRoles above).
// Checking for "any role at all" is not enough: a typical Keycloak human
// token already carries default-roles-<realm> and offline_access and would
// pass that weaker check, then still hit permission denied on the gateway.
func PreflightWarnings(tok *Token, wantAudience string) []string {
	if tok == nil {
		return nil
	}
	var warnings []string
	if wantAudience != "" && !slices.Contains(tok.Audience, wantAudience) {
		warnings = append(warnings, fmt.Sprintf(
			"token audience %v does not include the expected audience %q", tok.Audience, wantAudience))
	}
	if !slices.ContainsFunc(gatewayRoles, func(r string) bool { return slices.Contains(tok.Roles, r) }) {
		warnings = append(warnings, fmt.Sprintf(
			"token roles %v include neither %q nor %q; gateway calls will likely fail with permission denied",
			tok.Roles, gatewayRoles[0], gatewayRoles[1]))
	}
	return warnings
}
