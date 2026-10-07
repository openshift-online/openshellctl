package gatewayconfig

import "strings"

// activeGatewayRel is the path (relative to the user config root) of the
// active_gateway pointer file — config-root level, not under gateways/ (same
// path readActive reads in config.go).
const activeGatewayRel = "active_gateway"

// knownGatewayFiles are every file gateway add/login/token-refresh may create
// under a gateway's directory. RemoveGateway removes each (a missing file is
// a no-op, per the Writer contract) rather than requiring a directory-walk
// method on the Writer interface.
var knownGatewayFiles = []string{
	"metadata.json",
	"oidc_token.json",
	"last_sandbox",
	"mtls/ca.crt",
	"mtls/tls.crt",
	"mtls/tls.key",
}

// SetActive writes name to the active_gateway pointer file.
func SetActive(w Writer, name string) error {
	if err := ValidateGatewayName(name); err != nil {
		return err
	}
	return w.WriteFile(activeGatewayRel, []byte(name), 0o600)
}

// ClearActiveIfMatches removes the active_gateway pointer when it currently
// names name (so removing or logging out of an unrelated, non-active gateway
// never clears it). A missing active_gateway file is a no-op. Mirrors
// ClearLastSandboxIfMatches's read-compare-remove idiom (lastsandbox.go).
func ClearActiveIfMatches(w Writer, name string) error {
	b, err := w.ReadFile(activeGatewayRel)
	if err != nil {
		return nil // nothing to clear
	}
	if strings.TrimSpace(string(b)) != name {
		return nil
	}
	return w.Remove(activeGatewayRel)
}

// RemoveGateway deletes every known on-disk file for a gateway registration
// (metadata, token, last_sandbox, mtls material). Each removal is a no-op if
// the file was never written — removing a gateway that only has a subset of
// these (e.g. never authenticated) is not an error.
func RemoveGateway(w Writer, name string) error {
	if err := ValidateGatewayName(name); err != nil {
		return err
	}
	for _, rel := range knownGatewayFiles {
		if err := w.Remove(GatewayDir(name) + "/" + rel); err != nil {
			return err
		}
	}
	return nil
}

// Logout removes only a gateway's oidc_token.json — de-authenticating while
// keeping the registration (metadata.json) intact, unlike RemoveGateway. A
// missing token file is a no-op.
func Logout(w Writer, name string) error {
	if err := ValidateGatewayName(name); err != nil {
		return err
	}
	return w.Remove(GatewayDir(name) + "/oidc_token.json")
}
