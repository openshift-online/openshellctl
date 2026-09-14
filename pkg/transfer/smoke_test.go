package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/gateway"
)

// These are end-to-end smoke tests over an in-process SSH server reached through
// the gateway.Gateway seam. They give confidence that the SSH/tar wiring holds
// together; the per-branch logic is covered by the pure tests. One happy-path per
// operation, plus the gateway-error path (which never touches SSH), is enough.

func TestSmokeUploadRoundTrip(t *testing.T) {
	src := t.TempDir()
	remote := t.TempDir()
	writeFile(t, filepath.Join(src, "proj", "a.txt"), "A")
	writeFile(t, filepath.Join(src, "proj", "sub", "b.txt"), "B")

	c, _ := newTestClient(t, src, remote)
	var reports []string
	if err := c.Upload(context.Background(), "default", "sb", "proj", "", false, func(s string) {
		reports = append(reports, s)
	}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	for rel, want := range map[string]string{"proj/a.txt": "A", "proj/sub/b.txt": "B"} {
		got, err := readFileAt(remote, rel)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q err %v, want %q", rel, got, err, want)
		}
	}
	if len(reports) < 2 || !strings.HasPrefix(reports[0], "Uploading ") || reports[len(reports)-1] != "✓ Upload complete" {
		t.Errorf("reports = %v", reports)
	}
}

func TestSmokeDownloadRoundTrip(t *testing.T) {
	src := t.TempDir()
	remote := t.TempDir()
	writeFile(t, filepath.Join(remote, "d", "x.txt"), "X")
	writeFile(t, filepath.Join(remote, "d", "sub", "y.txt"), "Y")

	c, _ := newTestClient(t, src, remote)
	dest := filepath.Join(t.TempDir(), "out")
	if err := c.Download(context.Background(), "default", "sb", "/ws/d", dest, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	for rel, want := range map[string]string{"x.txt": "X", "sub/y.txt": "Y"} {
		got, err := readFileAt(dest, rel)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q err %v want %q", rel, got, err, want)
		}
	}
}

func TestSmokeDownloadFile(t *testing.T) {
	src := t.TempDir()
	remote := t.TempDir()
	writeFile(t, filepath.Join(remote, "out.txt"), "downloaded")

	c, _ := newTestClient(t, src, remote)
	dest := filepath.Join(t.TempDir(), "local.txt")
	var reports []string
	if err := c.Download(context.Background(), "default", "sb", "/ws/out.txt", dest, func(s string) {
		reports = append(reports, s)
	}); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, err := readFileAt(filepath.Dir(dest), filepath.Base(dest)); err != nil || string(got) != "downloaded" {
		t.Errorf("dest = %q err %v", got, err)
	}
	if len(reports) < 2 || reports[len(reports)-1] != "✓ Download complete" {
		t.Errorf("reports = %v", reports)
	}
}

func TestSmokeDownloadMissing(t *testing.T) {
	c, _ := newTestClient(t, t.TempDir(), t.TempDir())
	err := c.Download(context.Background(), "default", "sb", "/ws/nope", filepath.Join(t.TempDir(), "x"), nil)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("want does-not-exist error, got %v", err)
	}
}

func TestSmokeDownloadDirOverFileFails(t *testing.T) {
	src := t.TempDir()
	remote := t.TempDir()
	writeFile(t, filepath.Join(remote, "d", "x.txt"), "X")

	c, _ := newTestClient(t, src, remote)
	dest := filepath.Join(t.TempDir(), "afile")
	writeFile(t, dest, "existing")
	err := c.Download(context.Background(), "default", "sb", "/ws/d", dest, nil)
	if err == nil || !strings.Contains(err.Error(), "non-directory destination") {
		t.Fatalf("want non-directory error, got %v", err)
	}
}

func TestSmokeDownloadFileOverDirFails(t *testing.T) {
	src := t.TempDir()
	remote := t.TempDir()
	writeFile(t, filepath.Join(remote, "f.txt"), "content")

	c, _ := newTestClient(t, src, remote)
	destDir := t.TempDir()
	// A directory already occupies the final file path → cannot overwrite.
	writeFile(t, filepath.Join(destDir, "f.txt", "child"), "x")
	err := c.Download(context.Background(), "default", "sb", "/ws/f.txt", destDir, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot overwrite directory") {
		t.Fatalf("want overwrite-directory error, got %v", err)
	}
}

func TestSmokeConnectExitCode(t *testing.T) {
	c, gw := newTestClient(t, t.TempDir(), t.TempDir())
	gw.mainReply = "hello from sandbox\n"
	gw.mainExit = 7

	var out bytes.Buffer
	code, err := c.Connect(context.Background(), "default", "sb", false,
		NopTerminal{In: strings.NewReader(""), Out: &out, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
	if !strings.Contains(out.String(), "hello from sandbox") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestSmokeConnectDetach(t *testing.T) {
	c, gw := newTestClient(t, t.TempDir(), t.TempDir())
	gw.mainExit = 42   // would be the code if we didn't detach first
	gw.mainHold = true // keep the session live so the detach chord wins deterministically

	code, err := c.Connect(context.Background(), "default", "sb", false,
		NopTerminal{In: bytes.NewReader([]byte{ctrlP, ctrlQ}), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if code != 0 {
		t.Errorf("detach exit code = %d, want 0", code)
	}
}

// tunnelErrGateway fails every SSHTunnel — exercises the gateway-error path that
// each operation must surface before any SSH work.
type tunnelErrGateway struct{ gateway.Gateway }

func (tunnelErrGateway) SSHTunnel(context.Context, string, string) (io.ReadWriteCloser, error) {
	return nil, errors.New("tunnel refused")
}

func TestSmokeGatewayTunnelError(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "x")
	c := &client{gw: tunnelErrGateway{}, fsys: OSFS(src)}
	ctx := context.Background()

	if err := c.Upload(ctx, "default", "sb", "a.txt", "", false, nil); err == nil || !strings.Contains(err.Error(), "tunnel refused") {
		t.Errorf("upload tunnel err = %v", err)
	}
	if err := c.Download(ctx, "default", "sb", "/ws/a.txt", filepath.Join(t.TempDir(), "o"), nil); err == nil || !strings.Contains(err.Error(), "tunnel refused") {
		t.Errorf("download tunnel err = %v", err)
	}
	if _, err := c.Connect(ctx, "default", "sb", false, NopTerminal{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}); err == nil || !strings.Contains(err.Error(), "tunnel refused") {
		t.Errorf("connect tunnel err = %v", err)
	}
}

// TestSmokeUploadLocalMissing covers the pre-SSH stat error (a real branch in
// the Upload shell that the pure tests can't reach).
func TestSmokeUploadLocalMissing(t *testing.T) {
	c, _ := newTestClient(t, t.TempDir(), t.TempDir())
	err := c.Upload(context.Background(), "default", "sb", "nope.txt", "", false, nil)
	if err == nil || err.Error() != "local path does not exist: nope.txt" {
		t.Fatalf("want local-missing error, got %v", err)
	}
}
