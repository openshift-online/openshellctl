package gatewayconfig

import (
	"io/fs"
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

func resolvedWithLastSandbox(content string) *Resolved {
	return &Resolved{
		Name: "rosa",
		FS:   fstest.MapFS{"last_sandbox": {Data: []byte(content)}},
	}
}

func TestLoadLastSandbox_Match(t *testing.T) {
	r := resolvedWithLastSandbox("default\nsb-123")
	name, ok := LoadLastSandbox(r, "default")
	if !ok || name != "sb-123" {
		t.Errorf("got (%q,%v), want (sb-123,true)", name, ok)
	}
}

func TestLoadLastSandbox_WorkspaceMismatch(t *testing.T) {
	r := resolvedWithLastSandbox("other\nsb-123")
	if _, ok := LoadLastSandbox(r, "default"); ok {
		t.Errorf("expected miss on workspace mismatch")
	}
}

func TestLoadLastSandbox_LegacySingleLine(t *testing.T) {
	r := resolvedWithLastSandbox("sb-123")
	if _, ok := LoadLastSandbox(r, "default"); ok {
		t.Errorf("legacy single-line should miss")
	}
}

func TestLoadLastSandbox_Empty(t *testing.T) {
	r := resolvedWithLastSandbox("   \n")
	if _, ok := LoadLastSandbox(r, "default"); ok {
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
