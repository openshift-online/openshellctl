package output

import (
	"fmt"
	"io"
	"strings"

	"sigs.k8s.io/yaml"
)

// DoctorCheck is the minimal view RenderDoctor needs of one `doctor` check
// result. Defined here rather than imported from pkg/doctor (whose
// CheckResult this otherwise mirrors field-for-field) to avoid an import
// cycle: pkg/doctor imports pkg/sandbox (for ResolveProviders), and
// pkg/sandbox imports this package (for log formatting) — so pkg/output
// must not import pkg/doctor. internal/cli/doctor.go converts
// []doctor.CheckResult to []output.DoctorCheck at the one call site that
// needs both.
type DoctorCheck struct {
	Name     string
	Status   string // "pass" | "fail" | "skip" — doctor.Status's string form
	Detail   string
	NextStep string
}

// RenderDoctor writes `doctor`'s table/json/yaml view.
func RenderDoctor(w io.Writer, checks []DoctorCheck, format Format) error {
	switch format {
	case FormatJSON:
		return renderDoctorJSON(w, checks)
	case FormatYAML:
		return renderDoctorYAML(w, checks)
	default:
		return renderDoctorTable(w, checks)
	}
}

// renderDoctorTable writes one line per check: a glyph, the check name, and
// its detail; a failing check gets an additional indented "Hint:" line with
// NextStep. Skipped checks render with "-", not a symbol that could be
// mistaken for pass/fail.
func renderDoctorTable(w io.Writer, checks []DoctorCheck) error {
	if len(checks) == 0 {
		_, err := fmt.Fprintln(w, "No checks run.")
		return err
	}

	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Name
	}
	width := maxLen(names, 4)

	var b strings.Builder
	for _, c := range checks {
		fmt.Fprintf(&b, "%s %s  %s\n", doctorGlyph(c.Status), pad(c.Name, width), c.Detail)
		if c.Status == "fail" && c.NextStep != "" {
			fmt.Fprintf(&b, "  Hint: %s\n", c.NextStep)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func doctorGlyph(status string) string {
	switch status {
	case "pass":
		return "✓"
	case "fail":
		return "✗"
	default:
		return "-"
	}
}

// doctorCheckMap is the sorted-key map used for the JSON/YAML views.
// NextStep is always present (as "" when unset), matching the rest of this
// package's convention (e.g. gatewayMap) of a stable key set regardless of
// which fields happen to be empty.
func doctorCheckMap(c DoctorCheck) map[string]any {
	return map[string]any{
		"detail":   c.Detail,
		"name":     c.Name,
		"nextStep": c.NextStep,
		"status":   c.Status,
	}
}

func renderDoctorJSON(w io.Writer, checks []DoctorCheck) error {
	maps := make([]map[string]any, len(checks))
	for i, c := range checks {
		maps[i] = doctorCheckMap(c)
	}
	data, err := marshalSortedJSON(maps)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func renderDoctorYAML(w io.Writer, checks []DoctorCheck) error {
	maps := make([]map[string]any, len(checks))
	for i, c := range checks {
		maps[i] = doctorCheckMap(c)
	}
	data, err := yaml.Marshal(maps)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
