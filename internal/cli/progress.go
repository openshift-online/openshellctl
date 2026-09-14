package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/openshift-online/openshellctl/pkg/sandbox"
)

// plainSink is a ProgressSink that writes plain (non-interactive) progress lines
// to a writer. It is the default sink; interactive/spinner rendering can be
// added later.
type plainSink struct{ w io.Writer }

func newPlainSink(w io.Writer) sandbox.ProgressSink { return &plainSink{w: w} }

func (s *plainSink) Header(name string) {
	_, _ = fmt.Fprintf(s.w, "Created sandbox: %s\n", name)
}

func (s *plainSink) StepDone(label string, elapsed time.Duration) {
	_, _ = fmt.Fprintf(s.w, "  ✓ %s (%s)\n", label, elapsed.Round(time.Second))
}

func (s *plainSink) StepActive(label, detail string) {
	if detail != "" {
		_, _ = fmt.Fprintf(s.w, "  %s %s\n", label, detail)
	} else {
		_, _ = fmt.Fprintf(s.w, "  %s\n", label)
	}
}
func (s *plainSink) Warning(msg string) { _, _ = fmt.Fprintf(s.w, "  ! %s\n", msg) }
func (s *plainSink) Error(msg string)   { _, _ = fmt.Fprintf(s.w, "  ✗ %s\n", msg) }

// provisionTimeout reads OPENSHELL_PROVISION_TIMEOUT (seconds), default 300s.
func provisionTimeout() time.Duration {
	return envSecondsDefault("OPENSHELL_PROVISION_TIMEOUT", 300*time.Second)
}
