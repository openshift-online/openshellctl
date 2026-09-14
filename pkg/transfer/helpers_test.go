package transfer

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFileAt(dir, rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
}

func readlinkAt(dir, rel string) (string, error) {
	return os.Readlink(filepath.Join(dir, filepath.FromSlash(rel)))
}

// symlinkOrSkip creates a symlink, skipping the test if the platform disallows
// it. Returns a non-nil error only when the caller should stop (already skipped).
func symlinkOrSkip(t *testing.T, target, link string) error {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
		return err
	}
	return nil
}

func mkfifoOrSkip(t *testing.T, path string) error {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
		return err
	}
	return nil
}

// readTarNames returns "name|typeflag|linkname" for each tar entry in order.
func readTarNames(t *testing.T, r io.Reader) []string {
	t.Helper()
	var names []string
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		names = append(names, fmt.Sprintf("%s|%c|%s", h.Name, h.Typeflag, h.Linkname))
	}
	return names
}
