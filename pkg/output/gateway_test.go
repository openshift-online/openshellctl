package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

func gw(name, endpoint, typ, auth string, active bool) gatewayconfig.DetailedInfo {
	return gatewayconfig.DetailedInfo{
		Info:     gatewayconfig.Info{Name: name, Source: gatewayconfig.SourceUser, Active: active},
		Endpoint: endpoint,
		Type:     typ,
		Auth:     auth,
	}
}

func TestRenderGatewayList_Empty(t *testing.T) {
	var b bytes.Buffer
	if err := RenderGatewayList(&b, nil, FormatTable); err != nil {
		t.Fatal(err)
	}
	if b.String() != "No gateways found.\n" {
		t.Errorf("empty list = %q", b.String())
	}
}

func TestRenderGatewayList_Table(t *testing.T) {
	var b bytes.Buffer
	gateways := []gatewayconfig.DetailedInfo{
		gw("test-gw", "https://gw.example.com:443", "remote", "oidc", true),
		gw("other", "https://other.example.com", "remote", "none", false),
	}
	if err := RenderGatewayList(&b, gateways, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "NAME") {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(lines[0], "AUTH") {
		t.Errorf("header missing AUTH column: %q", lines[0])
	}
	var activeLine, otherLine string
	for _, l := range lines[1:] {
		if strings.Contains(l, "test-gw") {
			activeLine = l
		}
		if strings.Contains(l, "other") {
			otherLine = l
		}
	}
	if !strings.HasPrefix(activeLine, "*") {
		t.Errorf("active row should be prefixed with *, got: %q", activeLine)
	}
	if !strings.Contains(activeLine, "oidc") {
		t.Errorf("active row missing AUTH=oidc: %q", activeLine)
	}
	if strings.HasPrefix(otherLine, "*") {
		t.Errorf("non-active row should not be prefixed with *, got: %q", otherLine)
	}
}

func TestRenderGatewayList_JSON(t *testing.T) {
	var b bytes.Buffer
	gateways := []gatewayconfig.DetailedInfo{gw("test-gw", "https://gw", "remote", "oidc", true)}
	if err := RenderGatewayList(&b, gateways, FormatJSON); err != nil {
		t.Fatal(err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal(b.Bytes(), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, b.String())
	}
	if len(parsed) != 1 || parsed[0]["name"] != "test-gw" || parsed[0]["auth"] != "oidc" {
		t.Errorf("parsed = %+v", parsed)
	}
	if parsed[0]["active"] != true {
		t.Errorf("active = %v, want true", parsed[0]["active"])
	}
}

func TestRenderGatewayList_YAML(t *testing.T) {
	var b bytes.Buffer
	gateways := []gatewayconfig.DetailedInfo{gw("test-gw", "https://gw", "remote", "oidc", true)}
	if err := RenderGatewayList(&b, gateways, FormatYAML); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "name: test-gw") || !strings.Contains(out, "auth: oidc") {
		t.Errorf("yaml = %s", out)
	}
	if strings.Contains(out, "---") {
		t.Error("YAML should not contain a document separator")
	}
}
