package gatewayconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSWriter_AtomicWriteAndPerms(t *testing.T) {
	root := t.TempDir()
	w := &OSWriter{Root: root}

	if err := w.WriteFile("gateways/rosa/last_sandbox", []byte("default\nsb-1"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	full := filepath.Join(root, "gateways", "rosa", "last_sandbox")
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(b) != "default\nsb-1" {
		t.Errorf("content = %q", string(b))
	}
	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file perm = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Join(root, "gateways", "rosa"))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir perm = %o, want 700", dirInfo.Mode().Perm())
	}
}

func TestOSWriter_RemoveMissingIsNoError(t *testing.T) {
	w := &OSWriter{Root: t.TempDir()}
	if err := w.Remove("gateways/x/last_sandbox"); err != nil {
		t.Errorf("Remove missing = %v, want nil", err)
	}
}

func TestOSWriter_ReadRoundTrip(t *testing.T) {
	w := &OSWriter{Root: t.TempDir()}
	_ = w.WriteFile("f", []byte("hi"), 0o600)
	b, err := w.ReadFile("f")
	if err != nil || string(b) != "hi" {
		t.Errorf("ReadFile = %q, %v", string(b), err)
	}
}
