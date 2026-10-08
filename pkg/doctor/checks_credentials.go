package doctor

import (
	"errors"
	"fmt"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

// CheckCredentials classifies the outcome of actually minting a token — the
// I/O (auth.Resolve + TokenSource.Token) happens once in Run, and this
// function is a pure classifier over the result, so every branch is
// table-testable without a real resolve/mint.
//
// When err wraps *auth.ErrNoCredentials, its Checked list (Feature B) is
// surfaced verbatim in Detail — this is the "credentials check shows the
// checked-sources list" acceptance criterion, free, because ErrNoCredentials
// already carries that list.
func CheckCredentials(tok *auth.Token, err error) CheckResult {
	if err != nil {
		var noCreds *auth.ErrNoCredentials
		if errors.As(err, &noCreds) {
			return CheckResult{
				Name:   "Credentials",
				Status: StatusFail,
				Detail: err.Error(),
				NextStep: "for a service account, export OPENSHELL_OIDC_CLIENT_SECRET (or pass --client-secret-file); " +
					"for a human, run `openshellctl login`",
			}
		}
		return CheckResult{
			Name:     "Credentials",
			Status:   StatusFail,
			Detail:   err.Error(),
			NextStep: "resolve the error above, then re-run `openshellctl doctor`",
		}
	}
	if tok == nil {
		return CheckResult{
			Name: "Credentials", Status: StatusFail, Detail: "no token minted",
			NextStep: "re-run `openshellctl doctor`",
		}
	}
	return CheckResult{
		Name:   "Credentials",
		Status: StatusPass,
		Detail: fmt.Sprintf("minted token for subject %q", tok.Subject),
	}
}

// errNotWired is CheckCredentials' error detail when Deps.ResolveAuth itself
// is nil — a defensive, not a real-world, path: internal/cli/doctor.go
// always wires a real resolveAuth. Exercised by TestRun_NilDeps.
var errNotWired = errors.New("not wired")
