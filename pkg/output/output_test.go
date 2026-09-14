package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

func sb(name, ws, phase string) *types.Sandbox {
	return &types.Sandbox{
		ID:        "id-" + name,
		Name:      name,
		Workspace: ws,
		CreatedAt: time.UnixMilli(1700000000000),
		Status:    types.SandboxStatus{Phase: types.SandboxPhase(phase)},
	}
}

func TestFormatEpochMs(t *testing.T) {
	if got := FormatEpochMs(-1); got != "-" {
		t.Errorf("negative = %q, want -", got)
	}
	if got := FormatEpochMs(0); got != "1970-01-01 00:00:00" {
		t.Errorf("zero = %q", got)
	}
	if got := FormatEpochMs(1700000000000); got != "2023-11-14 22:13:20" {
		t.Errorf("epoch = %q", got)
	}
}

func TestRenderSandboxJSON_SortedKeys(t *testing.T) {
	var b bytes.Buffer
	if err := RenderSandboxJSON(&b, sb("s1", "default", "Ready")); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	// Verify it is valid JSON and keys appear in sorted order.
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	wantKeys := []string{"annotations", "created_at", "current_policy_version", "id", "labels", "name", "phase", "resource_version", "workspace"}
	for _, k := range wantKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	// Sorted order check: each key's index in the raw string is increasing.
	last := -1
	for _, k := range wantKeys {
		idx := strings.Index(out, `"`+k+`"`)
		if idx < last {
			t.Errorf("key %q out of sorted order", k)
		}
		last = idx
	}
}

func TestRenderSandboxYAML(t *testing.T) {
	var b bytes.Buffer
	if err := RenderSandboxYAML(&b, sb("s1", "default", "Ready")); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "---") {
		t.Error("YAML should not contain document separator")
	}
	if !strings.Contains(out, "name: s1") || !strings.Contains(out, "phase: Ready") {
		t.Errorf("yaml = %s", out)
	}
}

func TestRenderSandboxList_Empty(t *testing.T) {
	var b bytes.Buffer
	if err := RenderSandboxList(&b, nil, ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if b.String() != "No sandboxes found.\n" {
		t.Errorf("empty list = %q", b.String())
	}
}

func TestRenderSandboxList_Table(t *testing.T) {
	var b bytes.Buffer
	sandboxes := []*types.Sandbox{sb("short", "default", "Ready"), sb("a-longer-name", "default", "Error")}
	if err := RenderSandboxList(&b, sandboxes, ListOptions{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "NAME") {
		t.Errorf("header = %q", lines[0])
	}
	// Two-space separators and left-aligned name column width = len("a-longer-name").
	if !strings.Contains(out, "a-longer-name  ") {
		t.Errorf("expected padded name column:\n%s", out)
	}
}

func TestRenderSandboxList_AllWorkspaces(t *testing.T) {
	var b bytes.Buffer
	if err := RenderSandboxList(&b, []*types.Sandbox{sb("s", "prod", "Ready")}, ListOptions{AllWorkspaces: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "WORKSPACE") {
		t.Errorf("all-workspaces header missing: %q", b.String())
	}
}

func TestRenderSandboxIDsAndNames(t *testing.T) {
	sandboxes := []*types.Sandbox{sb("s1", "default", "Ready"), sb("s2", "prod", "Ready")}
	var ids bytes.Buffer
	_ = RenderSandboxIDs(&ids, sandboxes)
	if ids.String() != "id-s1\nid-s2\n" {
		t.Errorf("ids = %q", ids.String())
	}
	var names bytes.Buffer
	_ = RenderSandboxNames(&names, sandboxes, false)
	if names.String() != "s1\ns2\n" {
		t.Errorf("names = %q", names.String())
	}
	var qnames bytes.Buffer
	_ = RenderSandboxNames(&qnames, sandboxes, true)
	if qnames.String() != "default/s1\nprod/s2\n" {
		t.Errorf("qualified names = %q", qnames.String())
	}
}

func TestRenderSandboxGetTable(t *testing.T) {
	s := sb("s1", "default", "Ready")
	s.ResourceVersion = 42
	s.Labels = map[string]string{"b": "2", "a": "1"}
	var b bytes.Buffer
	if err := RenderSandboxGetTable(&b, s); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Sandbox:\n") || !strings.Contains(out, "  Id: id-s1") {
		t.Errorf("get table = %s", out)
	}
	if !strings.Contains(out, "  Resource version: 42") {
		t.Errorf("missing resource version: %s", out)
	}
	// Labels sorted.
	ai := strings.Index(out, "a: 1")
	bi := strings.Index(out, "b: 2")
	if ai < 0 || bi < 0 || ai > bi {
		t.Errorf("labels not sorted: %s", out)
	}
}

func TestRenderProviderList(t *testing.T) {
	var empty bytes.Buffer
	_ = RenderProviderList(&empty, "sb-1", nil)
	if empty.String() != "No providers attached to sandbox sb-1.\n" {
		t.Errorf("empty = %q", empty.String())
	}

	provs := []*types.Provider{
		{Name: "gh", Type: "github", Spec: types.ProviderSpec{
			Credentials: map[string]string{"api_token": "x"},
			Config:      map[string]string{"org": "y"},
		}},
	}
	var b bytes.Buffer
	_ = RenderProviderList(&b, "sb-1", provs)
	out := b.String()
	if !strings.HasPrefix(out, "NAME") || !strings.Contains(out, "github") {
		t.Errorf("provider table = %s", out)
	}
}

func TestFormatLogLine(t *testing.T) {
	l := types.LogLine{
		Timestamp: time.UnixMilli(5432),
		Level:     "INFO",
		Target:    "supervisor",
		Message:   "started",
		Source:    "vm",
		Fields:    map[string]string{"b": "2", "a": "1"},
	}
	got := FormatLogLine(l)
	if !strings.HasPrefix(got, "[5.432] [vm     ] [INFO ] [supervisor] started") {
		t.Errorf("log line = %q", got)
	}
	if !strings.HasSuffix(got, "a=1 b=2") {
		t.Errorf("fields not sorted/appended: %q", got)
	}
}

func TestFormatLogLine_EmptySourceIsGateway(t *testing.T) {
	l := types.LogLine{Timestamp: time.UnixMilli(0), Level: "WARN", Message: "x"}
	got := FormatLogLine(l)
	if !strings.Contains(got, "[gateway]") {
		t.Errorf("empty source should render gateway: %q", got)
	}
}
