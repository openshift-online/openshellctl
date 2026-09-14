// Package output renders sandboxes/providers/logs in table/json/yaml formats
// byte-matched to the upstream CLI (spec §5.8, Appendix A.1-A.2). JSON/YAML use
// the CLI's sorted key order; tables use the exact headers/widths/separators.
package output

import (
	"sort"
	"strings"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

// Format is the output format.
type Format string

// Output formats.
const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// FormatEpochMs renders a millisecond epoch as "YYYY-MM-DD HH:MM:SS" UTC.
// Negative → "-"; 0 → the Unix epoch (common.rs:72-98). This is the list/get
// JSON variant (0 renders as the epoch, not "-").
func FormatEpochMs(ms int64) string {
	if ms < 0 {
		return "-"
	}
	t := time.UnixMilli(ms).UTC()
	return t.Format("2006-01-02 15:04:05")
}

// formatCreatedAt renders a time.Time as the CLI created_at string.
func formatCreatedAt(t time.Time) string {
	if t.IsZero() {
		return FormatEpochMs(0)
	}
	return FormatEpochMs(t.UnixMilli())
}

// sandboxMap builds the sorted-key map used for list/create JSON+YAML
// (sandbox_to_json, run.rs:2090-2108).
func sandboxMap(s *types.Sandbox) map[string]any {
	return map[string]any{
		"annotations":            emptyMapIfNil(s.Annotations),
		"created_at":             formatCreatedAt(s.CreatedAt),
		"current_policy_version": s.Status.CurrentPolicyVersion,
		"id":                     s.ID,
		"labels":                 emptyMapIfNil(s.Labels),
		"name":                   s.Name,
		"phase":                  string(s.Status.Phase),
		"resource_version":       s.ResourceVersion,
		"workspace":              s.Workspace,
	}
}

func emptyMapIfNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// sortedStringKeys returns map keys sorted.
func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// pad left-aligns s to width.
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// maxLen returns the max length across values and a floor.
func maxLen(values []string, floor int) int {
	w := floor
	for _, v := range values {
		if len(v) > w {
			w = len(v)
		}
	}
	return w
}
