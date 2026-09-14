package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"sigs.k8s.io/yaml"
)

// marshalSortedJSON emits 2-space-indented JSON with keys in sorted order plus a
// trailing newline (serde_json::to_string_pretty of a BTreeMap). HTML escaping
// is disabled to match serde.
func marshalSortedJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil // Encoder already appends a newline
}

// RenderSandboxJSON writes the list/create JSON view of one sandbox.
func RenderSandboxJSON(w io.Writer, s *types.Sandbox) error {
	data, err := marshalSortedJSON(sandboxMap(s))
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// RenderSandboxYAML writes the YAML view (no "---", sorted keys).
func RenderSandboxYAML(w io.Writer, s *types.Sandbox) error {
	data, err := yaml.Marshal(sandboxMap(s))
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ListOptions controls list-table rendering.
type ListOptions struct {
	AllWorkspaces bool
	Color         bool
}

// RenderSandboxList writes the `sandbox list` table (run.rs:2021-2085). Empty →
// "No sandboxes found.".
func RenderSandboxList(w io.Writer, sandboxes []*types.Sandbox, opts ListOptions) error {
	if len(sandboxes) == 0 {
		_, err := fmt.Fprintln(w, "No sandboxes found.")
		return err
	}

	names := make([]string, len(sandboxes))
	workspaces := make([]string, len(sandboxes))
	for i, s := range sandboxes {
		names[i] = s.Name
		workspaces[i] = s.Workspace
	}
	nameWidth := maxLen(names, 4)
	const createdWidth = 19
	wsWidth := maxLen(workspaces, 9)

	var b strings.Builder
	if opts.AllWorkspaces {
		fmt.Fprintf(&b, "%s  %s  %s  %s\n", pad("WORKSPACE", wsWidth), pad("NAME", nameWidth), pad("CREATED", createdWidth), "PHASE")
	} else {
		fmt.Fprintf(&b, "%s  %s  %s\n", pad("NAME", nameWidth), pad("CREATED", createdWidth), "PHASE")
	}
	for _, s := range sandboxes {
		created := formatCreatedAt(s.CreatedAt)
		phase := string(s.Status.Phase)
		if opts.AllWorkspaces {
			fmt.Fprintf(&b, "%s  %s  %s  %s\n", pad(s.Workspace, wsWidth), pad(s.Name, nameWidth), pad(created, createdWidth), phase)
		} else {
			fmt.Fprintf(&b, "%s  %s  %s\n", pad(s.Name, nameWidth), pad(created, createdWidth), phase)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderSandboxIDs prints ids one per line.
func RenderSandboxIDs(w io.Writer, sandboxes []*types.Sandbox) error {
	var b strings.Builder
	for _, s := range sandboxes {
		b.WriteString(s.ID)
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderSandboxNames prints names (or <workspace>/<name> with allWorkspaces).
func RenderSandboxNames(w io.Writer, sandboxes []*types.Sandbox, allWorkspaces bool) error {
	var b strings.Builder
	for _, s := range sandboxes {
		if allWorkspaces {
			fmt.Fprintf(&b, "%s/%s\n", s.Workspace, s.Name)
		} else {
			b.WriteString(s.Name)
			b.WriteByte('\n')
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderSandboxGetTable writes the `sandbox get` detail table (run.rs:1314-1390).
func RenderSandboxGetTable(w io.Writer, s *types.Sandbox) error {
	var b strings.Builder
	b.WriteString("Sandbox:\n\n")
	fmt.Fprintf(&b, "  Id: %s\n", orUnknown(s.ID))
	fmt.Fprintf(&b, "  Name: %s\n", orUnknown(s.Name))
	fmt.Fprintf(&b, "  Phase: %s\n", string(s.Status.Phase))
	fmt.Fprintf(&b, "  Resource version: %d\n", s.ResourceVersion)
	if len(s.Labels) > 0 {
		b.WriteString("  Labels:\n")
		for _, k := range sortedStringKeys(s.Labels) {
			fmt.Fprintf(&b, "    %s: %s\n", k, s.Labels[k])
		}
	}
	if len(s.Annotations) > 0 {
		b.WriteString("  Annotations:\n")
		for _, k := range sortedStringKeys(s.Annotations) {
			fmt.Fprintf(&b, "    %s: %s\n", k, s.Annotations[k])
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
