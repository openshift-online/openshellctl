package auth

import (
	"fmt"
	"slices"
)

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
// A tok with no realm roles at all (tok.Roles empty) is also warned about,
// since gateway calls requiring any role will fail with permission denied —
// there is no "wanted role" parameter; this only asks whether any roles
// were granted at all, not whether a specific one was.
func PreflightWarnings(tok *Token, wantAudience string) []string {
	if tok == nil {
		return nil
	}
	var warnings []string
	if wantAudience != "" && !slices.Contains(tok.Audience, wantAudience) {
		warnings = append(warnings, fmt.Sprintf(
			"token audience %v does not include the expected audience %q", tok.Audience, wantAudience))
	}
	if len(tok.Roles) == 0 {
		warnings = append(warnings, "token carries no realm roles (realm_access.roles); "+
			"gateway calls requiring a role will likely fail with permission denied")
	}
	return warnings
}
