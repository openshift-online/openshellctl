package gatewayconfig

import (
	"io/fs"
	"path"
	"strings"
)

// Writer abstracts the few writes openshellctl performs (last_sandbox,
// oidc_token.json). Paths are relative to the USER config tree root. The real
// implementation writes atomically (temp + rename) with 0600 files and 0700
// parent dirs.
type Writer interface {
	WriteFile(relPath string, data []byte, perm fs.FileMode) error
	Remove(relPath string) error
	ReadFile(relPath string) ([]byte, error)
}

// lastSandboxRel is the path (relative to the user config root) of a gateway's
// last_sandbox file. It validates gatewayName so every caller — Load, Save, and
// Clear — goes through a single choke point, mirroring upstream's
// last_sandbox_path() / validated_gateway_name() pairing.
func lastSandboxRel(gatewayName string) (string, error) {
	if err := ValidateGatewayName(gatewayName); err != nil {
		return "", err
	}
	return path.Join(GatewayDir(gatewayName), "last_sandbox"), nil
}

// parseLastSandbox parses the two-line last_sandbox file content and returns
// the sandbox name when the stored workspace matches. Pure: no I/O.
func parseLastSandbox(raw, workspace string) (string, bool) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return "", false
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return "", false // legacy single-line
	}
	storedWorkspace := strings.TrimSpace(lines[0])
	sandbox := strings.TrimSpace(lines[1])
	if sandbox == "" || storedWorkspace != workspace {
		return "", false
	}
	return sandbox, true
}

// LoadLastSandbox returns the last-used sandbox name for a workspace. It reads
// gateways/<gatewayName>/last_sandbox from userFS (the USER config tree),
// matching the tree that SaveLastSandbox writes to. Using userFS unconditionally
// — rather than the gateway's resolved FS — means system-tree gateways (whose
// resolved FS is under /etc/openshell) correctly pick up pointers written by
// SaveLastSandbox into the user tree. The file format is "<workspace>\n<sandbox>";
// the name is returned only when the stored workspace matches. Legacy single-line
// files yield ("", false) (metadata.rs:282-324).
func LoadLastSandbox(userFS fs.FS, gatewayName, workspace string) (string, bool) {
	if userFS == nil {
		return "", false
	}
	rel, err := lastSandboxRel(gatewayName)
	if err != nil {
		return "", false
	}
	b, err := fs.ReadFile(userFS, rel)
	if err != nil {
		return "", false
	}
	return parseLastSandbox(string(b), workspace)
}

// validateLastSandboxField returns an error when v is empty or contains \n or \r.
func validateLastSandboxField(field, v string) error {
	if v == "" || strings.ContainsAny(v, "\n\r") {
		return &InvalidLastSandboxError{Field: field, Value: v}
	}
	return nil
}

// SaveLastSandbox writes "<workspace>\n<sandbox>" (no trailing newline) to the
// USER tree. The Writer is responsible for the 0700 parent dir and 0600 file.
// Returns *InvalidLastSandboxError when workspace or name is empty or contains
// a newline or carriage return.
func SaveLastSandbox(w Writer, gatewayName, workspace, name string) error {
	rel, err := lastSandboxRel(gatewayName)
	if err != nil {
		return err
	}
	if err := validateLastSandboxField("workspace", workspace); err != nil {
		return err
	}
	if err := validateLastSandboxField("name", name); err != nil {
		return err
	}
	content := workspace + "\n" + name
	return w.WriteFile(rel, []byte(content), 0o600)
}

// ClearLastSandboxIfMatches removes the last_sandbox file when it currently
// records the given workspace/name (so deleting an unrelated sandbox does not
// clear the pointer). A missing file is a no-op.
func ClearLastSandboxIfMatches(w Writer, gatewayName, workspace, name string) error {
	rel, err := lastSandboxRel(gatewayName)
	if err != nil {
		return err
	}
	b, err := w.ReadFile(rel)
	if err != nil {
		return nil // nothing to clear
	}
	content := strings.TrimSpace(string(b))
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return nil
	}
	if strings.TrimSpace(lines[0]) == workspace && strings.TrimSpace(lines[1]) == name {
		return w.Remove(rel)
	}
	return nil
}
