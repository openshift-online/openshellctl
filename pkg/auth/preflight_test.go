package auth

import "testing"

func TestPreflightWarnings_AudienceMatch_NoWarning(t *testing.T) {
	tok := &Token{Audience: []string{"openshell-cli", "other"}, Roles: []string{"openshell-user"}}
	warnings := PreflightWarnings(tok, "openshell-cli")
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

func TestPreflightWarnings_AudienceMismatch(t *testing.T) {
	// Simulates a token whose aud claim was a single string in the original
	// JWT (jwtinspect.go's audienceClaim already normalizes this to []string
	// before a Token is ever built, so PreflightWarnings just sees the slice
	// either way).
	tok := &Token{Audience: []string{"some-other-audience"}, Roles: []string{"openshell-user"}}
	warnings := PreflightWarnings(tok, "openshell-cli")
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
	if !contains(warnings[0], "openshell-cli") {
		t.Errorf("warning should name the expected audience, got: %q", warnings[0])
	}
}

func TestPreflightWarnings_AudienceWasOriginallyArray_StillMatches(t *testing.T) {
	// Simulates a token whose aud claim was a JSON array in the original JWT.
	tok := &Token{Audience: []string{"a", "openshell-cli", "b"}, Roles: []string{"openshell-user"}}
	warnings := PreflightWarnings(tok, "openshell-cli")
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none (audience present in the array)", warnings)
	}
}

func TestPreflightWarnings_NoWantAudience_SkipsAudienceCheck(t *testing.T) {
	tok := &Token{Audience: []string{"whatever"}, Roles: []string{"openshell-user"}}
	warnings := PreflightWarnings(tok, "")
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none when wantAudience is empty", warnings)
	}
}

func TestPreflightWarnings_NoRoles(t *testing.T) {
	tok := &Token{Audience: []string{"openshell-cli"}, Roles: nil}
	warnings := PreflightWarnings(tok, "openshell-cli")
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one (missing roles)", warnings)
	}
	if !contains(warnings[0], "role") {
		t.Errorf("warning should mention roles, got: %q", warnings[0])
	}
}

func TestPreflightWarnings_BothProblems(t *testing.T) {
	tok := &Token{Audience: []string{"wrong"}, Roles: nil}
	warnings := PreflightWarnings(tok, "openshell-cli")
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want exactly two", warnings)
	}
}

func TestPreflightWarnings_NoToken_ReturnsNil(t *testing.T) {
	if got := PreflightWarnings(nil, "openshell-cli"); got != nil {
		t.Errorf("PreflightWarnings(nil, ...) = %v, want nil", got)
	}
}
