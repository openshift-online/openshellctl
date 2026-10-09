package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

func prov(name, typ string) *types.Provider {
	return &types.Provider{
		ID:        "id-" + name,
		Name:      name,
		Type:      typ,
		Workspace: "default",
		CreatedAt: time.UnixMilli(1700000000000),
		Spec: types.ProviderSpec{
			Credentials: map[string]string{"TOKEN": "secret"},
			Config:      map[string]string{"org": "acme"},
		},
	}
}

func TestRenderProviders_EmptyTable(t *testing.T) {
	var b bytes.Buffer
	if err := RenderProviders(&b, nil, FormatTable); err != nil {
		t.Fatal(err)
	}
	if b.String() != "No providers found.\n" {
		t.Errorf("empty list = %q", b.String())
	}
}

func TestRenderProviders_Table(t *testing.T) {
	var b bytes.Buffer
	providers := []*types.Provider{prov("gh", "github"), prov("a-longer-name", "s3")}
	if err := RenderProviders(&b, providers, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.HasPrefix(out, "NAME") {
		t.Errorf("header = %q", out)
	}
	if !strings.Contains(out, "github") || !strings.Contains(out, "a-longer-name") {
		t.Errorf("table missing rows: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("table must never print credential values: %s", out)
	}
}

func TestRenderProviders_JSONSortedKeysNoSecrets(t *testing.T) {
	var b bytes.Buffer
	if err := RenderProviders(&b, []*types.Provider{prov("gh", "github")}, FormatJSON); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	var list []map[string]any
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(list))
	}
	m := list[0]
	for _, k := range []string{"name", "type", "workspace", "credential_keys", "config", "created_at"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %+v", k, m)
		}
	}
	if strings.Contains(out, "secret") {
		t.Errorf("JSON must never print credential values: %s", out)
	}
	keys, _ := m["credential_keys"].([]any)
	if len(keys) != 1 || keys[0] != "TOKEN" {
		t.Errorf("credential_keys = %+v, want [TOKEN]", m["credential_keys"])
	}
}

func TestRenderProviders_YAML(t *testing.T) {
	var b bytes.Buffer
	if err := RenderProviders(&b, []*types.Provider{prov("gh", "github")}, FormatYAML); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "name: gh") {
		t.Errorf("yaml = %s", b.String())
	}
}

func TestRenderProvider_Table(t *testing.T) {
	var b bytes.Buffer
	if err := RenderProvider(&b, prov("gh", "github"), FormatTable); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Name:") || !strings.Contains(out, "gh") {
		t.Errorf("get table = %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("table must never print credential values: %s", out)
	}
}

func TestRenderProvider_YAML(t *testing.T) {
	var b bytes.Buffer
	if err := RenderProvider(&b, prov("gh", "github"), FormatYAML); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "name: gh") {
		t.Errorf("yaml = %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("yaml must never print credential values: %s", out)
	}
}

func TestRenderProvider_JSON(t *testing.T) {
	var b bytes.Buffer
	if err := RenderProvider(&b, prov("gh", "github"), FormatJSON); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b.String())
	}
	if m["name"] != "gh" {
		t.Errorf("name = %v, want gh", m["name"])
	}
}

func TestRenderProviderNames(t *testing.T) {
	var b bytes.Buffer
	providers := []*types.Provider{prov("gh", "github"), prov("s3", "s3")}
	if err := RenderProviderNames(&b, providers); err != nil {
		t.Fatal(err)
	}
	if b.String() != "gh\ns3\n" {
		t.Errorf("names = %q", b.String())
	}
}
