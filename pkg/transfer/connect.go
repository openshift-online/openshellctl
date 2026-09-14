package transfer

import (
	"context"
	"encoding/binary"
	"io"

	"golang.org/x/crypto/ssh"
)

// detach chord bytes: Ctrl-P (0x10) then Ctrl-Q (0x11).
const (
	ctrlP = 0x10
	ctrlQ = 0x11
)

// Terminal abstracts the local terminal for an interactive connect: putting the
// terminal into raw mode, reporting its size, and delivering resize events. A
// non-interactive caller passes NopTerminal.
type Terminal interface {
	// MakeRaw switches the terminal to raw mode, returning a restore func. When
	// the local endpoint is not a terminal it returns a no-op restore and false.
	MakeRaw() (restore func(), ok bool)
	// Size returns the current terminal size (cols, rows). Defaults 80x24.
	Size() (cols, rows int)
	// Stdin/Stdout/Stderr are the local streams to pump.
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
	// Resizes returns a channel that fires (best-effort) on terminal resize; nil
	// when unsupported. The returned stop func releases any resources.
	Resizes() (ch <-chan struct{}, stop func())
}

// ptyRequestMsg is the RFC 4254 §6.2 pty-req payload.
type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

type (
	subsystemRequestMsg struct{ Subsystem string }
	setenvRequest       struct{ Name, Value string }
	winchMsg            struct{ Columns, Rows, Width, Height uint32 }
)

// Connect attaches an interactive session to the sandbox's openshell-main
// subsystem and returns the remote exit code. It drives a raw session channel
// (rather than ssh.Session) so the exit status of a *subsystem* request is
// observable — ssh.Session.Wait only works for shell/exec, not subsystems.
// Mirrors ssh.rs:258-288 mapped onto the crypto/ssh channel API.
func (c *client) Connect(ctx context.Context, workspace, sandbox string, tty bool, term Terminal) (int, error) {
	cli, err := c.dialSSH(ctx, workspace, sandbox)
	if err != nil {
		return 0, err
	}
	defer func() { _ = cli.Close() }()

	ch, reqs, err := cli.OpenChannel("session", nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = ch.Close() }()

	// Collect the exit status from the channel's request stream. reqsDone closes
	// when the request stream ends (channel torn down), guaranteeing any
	// exit-status request has already been observed.
	exitCode := 0
	exitSet := false
	reqsDone := make(chan struct{})
	go func() {
		defer close(reqsDone)
		for req := range reqs {
			if req.Type == "exit-status" && len(req.Payload) >= 4 {
				exitCode = int(binary.BigEndian.Uint32(req.Payload))
				exitSet = true
			}
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}()

	if tty {
		cols, rows := termSize(term)
		payload := ssh.Marshal(&ptyRequestMsg{Term: "xterm-256color", Columns: uint32(cols), Rows: uint32(rows)}) //nolint:gosec // terminal dimensions are small non-negative values
		if _, err := ch.SendRequest("pty-req", true, payload); err != nil {
			return 0, err
		}
		if restore, ok := term.MakeRaw(); ok {
			defer restore()
		}
		if rc, stop := term.Resizes(); rc != nil {
			defer stop()
			go func() {
				for range rc {
					cols, rows := termSize(term)
					_, _ = ch.SendRequest("window-change", false, ssh.Marshal(&winchMsg{Columns: uint32(cols), Rows: uint32(rows)})) //nolint:gosec // small non-negative dimensions
				}
			}()
		}
	}

	// Mirror `-o SetEnv=TERM=xterm-256color`.
	_, _ = ch.SendRequest("env", false, ssh.Marshal(&setenvRequest{Name: "TERM", Value: "xterm-256color"}))

	if _, err := ch.SendRequest("subsystem", true, ssh.Marshal(&subsystemRequestMsg{Subsystem: "openshell-main"})); err != nil {
		return 0, err
	}

	// Pump remote stdout/stderr → local.
	outDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(term.Stdout(), ch)
		close(outDone)
	}()
	go func() { _, _ = io.Copy(term.Stderr(), ch.Stderr()) }()

	// Pump local stdin → remote, watching for the detach chord. On detach we tear
	// the whole channel down (mirroring the CLI's Ctrl-P Ctrl-Q disconnect) so the
	// session ends immediately and its exit status is abandoned; otherwise we just
	// signal EOF to the remote.
	detachCh := make(chan struct{}, 1)
	go func() {
		if pumpDetach(term.Stdin(), ch) {
			detachCh <- struct{}{}
			_ = ch.Close()
			return
		}
		_ = ch.CloseWrite()
	}()

	select {
	case <-detachCh:
		return 0, nil
	case <-outDone:
		// The remote closed its stdout. The exit-status request is delivered before
		// the channel is torn down, so wait for the request stream to drain (it ends
		// when the peer closes the channel). The deferred ch.Close handles cleanup.
		<-reqsDone
		if exitSet {
			return exitCode, nil
		}
		return 0, nil
	}
}

func termSize(term Terminal) (cols, rows int) {
	cols, rows = term.Size()
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	return cols, rows
}

// pumpDetach copies src→dst, returning true if the Ctrl-P Ctrl-Q chord was seen
// (the chord bytes are not forwarded). Byte-at-a-time is acceptable for
// interactive input volumes.
func pumpDetach(src io.Reader, dst io.Writer) bool {
	buf := make([]byte, 1)
	sawCtrlP := false
	for {
		n, err := src.Read(buf)
		if n > 0 {
			b := buf[0]
			switch {
			case sawCtrlP && b == ctrlQ:
				return true
			case sawCtrlP && b == ctrlP:
				// consecutive Ctrl-P: forward one, keep armed.
				if _, werr := dst.Write([]byte{ctrlP}); werr != nil {
					return false
				}
			case sawCtrlP:
				// Ctrl-P not followed by Ctrl-Q: forward the held Ctrl-P then this byte.
				if _, werr := dst.Write([]byte{ctrlP, b}); werr != nil {
					return false
				}
				sawCtrlP = false
			case b == ctrlP:
				sawCtrlP = true
			default:
				if _, werr := dst.Write([]byte{b}); werr != nil {
					return false
				}
			}
		}
		if err != nil {
			return false
		}
	}
}
