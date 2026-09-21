package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/transfer"
)

func TestParseUploadSpec(t *testing.T) {
	cases := []struct {
		in        string
		wantLocal string
		wantDest  string
	}{
		{"./mydir", "./mydir", ""},
		{"/abs/path:/remote/dest", "/abs/path", "/remote/dest"},
		{"local:dest", "local", "dest"},
		{"a:b:c", "a:b", "c"},
	}
	for _, c := range cases {
		local, dest := parseUploadSpec(c.in)
		if local != c.wantLocal || dest != c.wantDest {
			t.Errorf("parseUploadSpec(%q) = (%q, %q), want (%q, %q)", c.in, local, dest, c.wantLocal, c.wantDest)
		}
	}
}

func TestParseDownloadSpec(t *testing.T) {
	remote, dest := parseDownloadSpec("/remote/file:./local")
	if remote != "/remote/file" || dest != "./local" {
		t.Errorf("parseDownloadSpec = (%q, %q)", remote, dest)
	}
	remote, dest = parseDownloadSpec("/remote/file")
	if remote != "/remote/file" || dest != "" {
		t.Errorf("parseDownloadSpec no-dest = (%q, %q)", remote, dest)
	}
}

func TestResolveUploadFS_Relative(t *testing.T) {
	fsys, fsPath, err := resolveUploadFS("testdata")
	if err != nil {
		t.Fatalf("resolveUploadFS: %v", err)
	}
	if fsys == nil {
		t.Fatal("fsys is nil")
	}
	if fsPath != "testdata" {
		t.Errorf("fsPath = %q, want testdata", fsPath)
	}
}

func TestResolveUploadFS_Absolute(t *testing.T) {
	fsys, fsPath, err := resolveUploadFS("/tmp")
	if err != nil {
		t.Fatalf("resolveUploadFS: %v", err)
	}
	if fsys == nil {
		t.Fatal("fsys is nil")
	}
	if fsPath != "tmp" {
		t.Errorf("fsPath = %q, want tmp", fsPath)
	}
}

func TestSSHConfigBlock(t *testing.T) {
	target := &gatewayconfig.Target{Name: "mygw"}
	block := SSHConfigBlock("mysb", "default", target)

	for _, want := range []string{
		"Host openshell-mysb.default",
		"User sandbox",
		"StrictHostKeyChecking no",
		"UserKnownHostsFile /dev/null",
		"GlobalKnownHostsFile /dev/null",
		"LogLevel ERROR",
		"ServerAliveInterval 15",
		"ServerAliveCountMax 3",
		"--gateway-name mygw",
		"--name mysb",
		"--workspace default",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("SSHConfigBlock missing %q in:\n%s", want, block)
		}
	}
}

func TestSSHConfigBlock_NilTarget(t *testing.T) {
	block := SSHConfigBlock("sb1", "ws1", nil)
	if !strings.Contains(block, "--gateway-name default") {
		t.Errorf("nil target should fallback to 'default' gateway in:\n%s", block)
	}
	if !strings.Contains(block, "Host openshell-sb1.ws1") {
		t.Errorf("host alias wrong in:\n%s", block)
	}
}

// Verify wallClock satisfies transfer.Clock (compile-time check).
var _ transfer.Clock = wallClock{}

// Verify cliTerminal satisfies transfer.Terminal (compile-time check).
var _ transfer.Terminal = (*cliTerminal)(nil)

func TestCliTerminal_NotTTY(t *testing.T) {
	term := &cliTerminal{
		stdinFd:  -1,
		stdoutFd: -1,
		stdin:    strings.NewReader(""),
		stdout:   &strings.Builder{},
		stderr:   &strings.Builder{},
		isTTY:    false,
	}

	restore, ok := term.MakeRaw()
	if ok {
		t.Error("MakeRaw should return false for non-TTY")
	}
	restore()

	cols, rows := term.Size()
	if cols != 80 || rows != 24 {
		t.Errorf("Size = (%d, %d), want (80, 24) for bad fd", cols, rows)
	}

	if term.Stdin() == nil || term.Stdout() == nil || term.Stderr() == nil {
		t.Error("stream accessors should not return nil")
	}

	ch, stop := term.Resizes()
	if ch != nil {
		t.Error("Resizes should return nil channel for non-TTY")
	}
	stop()
}

func TestWallClock(t *testing.T) {
	c := wallClock{}
	now := c.Now()
	if now.IsZero() {
		t.Error("Now() should not be zero")
	}
	ch, stop := c.NewTicker(time.Second)
	if ch == nil {
		t.Error("NewTicker channel should not be nil")
	}
	stop()
}

func TestSSHConfigBlock_SpecialCharsEscaped(t *testing.T) {
	target := &gatewayconfig.Target{Name: "my gateway"}
	block := SSHConfigBlock("my sb", "ws", target)
	if !strings.Contains(block, "'my gateway'") {
		t.Errorf("gateway name not escaped in:\n%s", block)
	}
	if !strings.Contains(block, "'my sb'") {
		t.Errorf("sandbox name not escaped in:\n%s", block)
	}
}
