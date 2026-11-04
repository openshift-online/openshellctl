package gatewayconfig

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
)

// memWriter is an in-memory Writer for pure last_sandbox logic tests.
type memWriter struct{ files map[string][]byte }

func newMemWriter() *memWriter { return &memWriter{files: map[string][]byte{}} }

func (w *memWriter) WriteFile(rel string, data []byte, _ fs.FileMode) error {
	w.files[rel] = append([]byte(nil), data...)
	return nil
}
func (w *memWriter) Remove(rel string) error { delete(w.files, rel); return nil }
func (w *memWriter) ReadFile(rel string) ([]byte, error) {
	b, ok := w.files[rel]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

// userFSWithLastSandbox builds a MapFS whose last_sandbox sits under the
// gateway directory, matching the layout LoadLastSandbox now expects.
func userFSWithLastSandbox(gatewayName, content string) fs.FS {
	return fstest.MapFS{
		GatewayDir(gatewayName) + "/last_sandbox": {Data: []byte(content)},
	}
}

func TestLoadLastSandbox_Match(t *testing.T) {
	userFS := userFSWithLastSandbox("rosa", "default\nsb-123")
	name, ok := LoadLastSandbox(userFS, "rosa", "default")
	if !ok || name != "sb-123" {
		t.Errorf("got (%q,%v), want (sb-123,true)", name, ok)
	}
}

func TestLoadLastSandbox_WorkspaceMismatch(t *testing.T) {
	userFS := userFSWithLastSandbox("rosa", "other\nsb-123")
	if _, ok := LoadLastSandbox(userFS, "rosa", "default"); ok {
		t.Errorf("expected miss on workspace mismatch")
	}
}

func TestLoadLastSandbox_LegacySingleLine(t *testing.T) {
	userFS := userFSWithLastSandbox("rosa", "sb-123")
	if _, ok := LoadLastSandbox(userFS, "rosa", "default"); ok {
		t.Errorf("legacy single-line should miss")
	}
}

func TestLoadLastSandbox_Empty(t *testing.T) {
	userFS := userFSWithLastSandbox("rosa", "   \n")
	if _, ok := LoadLastSandbox(userFS, "rosa", "default"); ok {
		t.Errorf("empty file should miss")
	}
}

func TestSaveLastSandbox_Format(t *testing.T) {
	w := newMemWriter()
	if err := SaveLastSandbox(w, "rosa", "default", "sb-9"); err != nil {
		t.Fatal(err)
	}
	got := string(w.files["gateways/rosa/last_sandbox"])
	if got != "default\nsb-9" {
		t.Errorf("saved content = %q, want %q (no trailing newline)", got, "default\nsb-9")
	}
}

func TestSaveLastSandbox_RejectsNewlines(t *testing.T) {
	rows := []struct {
		name      string
		workspace string
		sbName    string
	}{
		{name: "name_newline", workspace: "w", sbName: "a\nb"},
		{name: "workspace_newline", workspace: "w\nx", sbName: "sb"},
		{name: "name_empty", workspace: "w", sbName: ""},
		{name: "name_cr", workspace: "w", sbName: "\r"},
		{name: "workspace_cr", workspace: "w\r", sbName: "sb"},
		{name: "workspace_empty", workspace: "", sbName: "sb"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			w := newMemWriter()
			err := SaveLastSandbox(w, "gw", row.workspace, row.sbName)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			var e *InvalidLastSandboxError
			if !errors.As(err, &e) {
				t.Fatalf("want *InvalidLastSandboxError, got %T: %v", err, err)
			}
			if len(w.files) != 0 {
				t.Errorf("memWriter should be empty on rejection, got %v", w.files)
			}
		})
	}
}

// TestLastSandbox_SystemGatewayRoundTrip verifies that SaveLastSandbox +
// LoadLastSandbox round-trip through the USER tree even when the gateway lives
// in the system tree. The old code read from r.FS (the gateway's sub-FS in
// SysFS), which missed files written by SaveLastSandbox into UserFS.
func TestLastSandbox_SystemGatewayRoundTrip(t *testing.T) {
	userDir := t.TempDir()
	env := Env{
		SysFS: fstest.MapFS{
			"gateways/sys/metadata.json": {Data: []byte(`{"endpoint":"http://sys"}`)},
		},
		UserFS: os.DirFS(userDir),
	}

	// Confirm "sys" is a system-tree gateway so the scenario is meaningful.
	resolved, err := Load(env, "sys")
	if err != nil || resolved.Source != SourceSystem {
		t.Fatalf("expected sys to be a system gateway: source=%v err=%v", resolved.Source, err)
	}

	w := &OSWriter{Root: userDir}
	if err := SaveLastSandbox(w, "sys", "default", "sb"); err != nil {
		t.Fatalf("save: %v", err)
	}

	// LoadLastSandbox must read from the user tree, not the system tree.
	name, ok := LoadLastSandbox(env.UserFS, "sys", "default")
	if !ok || name != "sb" {
		t.Errorf("round-trip: got (%q,%v), want (sb,true)", name, ok)
	}
}

func TestClearLastSandboxIfMatches(t *testing.T) {
	w := newMemWriter()
	_ = SaveLastSandbox(w, "rosa", "default", "sb-9")

	// Non-matching name: no clear.
	if err := ClearLastSandboxIfMatches(w, "rosa", "default", "other"); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.files["gateways/rosa/last_sandbox"]; !ok {
		t.Errorf("should not clear on non-matching name")
	}

	// Matching: clear.
	if err := ClearLastSandboxIfMatches(w, "rosa", "default", "sb-9"); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.files["gateways/rosa/last_sandbox"]; ok {
		t.Errorf("should clear on matching workspace+name")
	}

	// Missing file: no error.
	if err := ClearLastSandboxIfMatches(w, "rosa", "default", "sb-9"); err != nil {
		t.Errorf("clearing missing file should be a no-op, got %v", err)
	}
}
