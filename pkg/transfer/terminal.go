package transfer

import (
	"io"
)

// NopTerminal is a Terminal that never enters raw mode and reports no resizes.
// It uses the provided streams; Size is fixed at 80x24.
type NopTerminal struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// MakeRaw is a no-op for a non-interactive terminal.
func (t NopTerminal) MakeRaw() (func(), bool) { return func() {}, false }

// Size returns the fixed default size 80x24.
func (t NopTerminal) Size() (int, int) { return 80, 24 }

// Stdin returns the configured input stream.
func (t NopTerminal) Stdin() io.Reader { return t.In }

// Stdout returns the configured output stream.
func (t NopTerminal) Stdout() io.Writer { return t.Out }

// Stderr returns the configured error stream.
func (t NopTerminal) Stderr() io.Writer { return t.Err }

// Resizes reports no resize events.
func (t NopTerminal) Resizes() (<-chan struct{}, func()) { return nil, func() {} }
