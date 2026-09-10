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
// last_sandbox file.
func lastSandboxRel(gatewayName string) string {
	return path.Join(GatewayDir(gatewayName), "last_sandbox")
}

// LoadLastSandbox returns the last-used sandbox name for a workspace, reading
// gateways/<name>/last_sandbox from the gateway's sub-FS. The file is
// "<workspace>\n<sandbox>"; the name is returned only when the stored workspace
// matches. Legacy single-line files yield ("", false) (metadata.rs:282-324).
func LoadLastSandbox(r *Resolved, workspace string) (string, bool) {
	if r == nil || r.FS == nil {
		return "", false
	}
	b, err := fs.ReadFile(r.FS, "last_sandbox")
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(string(b))
	if content == "" {
		return "", false
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return "", false // legacy single-line
	}
	storedWorkspace := strings.TrimSpace(lines[0])
	sandbox := strings.TrimSpace(lines[1])
	if sandbox == "" {
		return "", false
	}
	if storedWorkspace != workspace {
		return "", false
	}
	return sandbox, true
}

// SaveLastSandbox writes "<workspace>\n<sandbox>" (no trailing newline) to the
// USER tree. The Writer is responsible for the 0700 parent dir and 0600 file.
func SaveLastSandbox(w Writer, gatewayName, workspace, name string) error {
	content := workspace + "\n" + name
	return w.WriteFile(lastSandboxRel(gatewayName), []byte(content), 0o600)
}

// ClearLastSandboxIfMatches removes the last_sandbox file when it currently
// records the given workspace/name (so deleting an unrelated sandbox does not
// clear the pointer). A missing file is a no-op.
func ClearLastSandboxIfMatches(w Writer, gatewayName, workspace, name string) error {
	rel := lastSandboxRel(gatewayName)
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
