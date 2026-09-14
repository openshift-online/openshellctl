package policyyaml

import (
	"strings"
	"testing"
)

func TestFromSDK_RoundTripStructure(t *testing.T) {
	y := `
version: 1
filesystem_policy:
  include_workdir: true
  read_only: ["/etc"]
process:
  run_as_user: sandbox
network_policies:
  httpbin:
    name: httpbin
    endpoints:
      - host: httpbin.org
        port: 443
        protocol: rest
        rules:
          - allow:
              method: POST
              path: /anything
    binaries:
      - path: /usr/bin/curl
`
	sdk, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	pf := FromSDK(sdk)
	if pf.Version != 1 {
		t.Errorf("version = %d", pf.Version)
	}
	if pf.FilesystemPolicy == nil || !pf.FilesystemPolicy.IncludeWorkdir {
		t.Error("filesystem lost in round-trip")
	}
	rule := pf.NetworkPolicies["httpbin"]
	if rule.Name != "httpbin" || len(rule.Endpoints) != 1 {
		t.Errorf("rule = %+v", rule)
	}
	e := rule.Endpoints[0]
	if e.Port != 443 || e.Protocol != "rest" {
		t.Errorf("endpoint = %+v", e)
	}
	if len(e.Rules) != 1 || e.Rules[0].Allow.Method != "POST" {
		t.Errorf("rules = %+v", e.Rules)
	}
	if len(rule.Binaries) != 1 || rule.Binaries[0].Path != "/usr/bin/curl" {
		t.Errorf("binaries = %+v", rule.Binaries)
	}
}

func TestFromSDK_EmptyProcessCollapses(t *testing.T) {
	y := "version: 1\nprocess:\n  run_as_user: \"\"\n"
	sdk, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	pf := FromSDK(sdk)
	if pf.Process != nil {
		t.Errorf("empty process should collapse to nil, got %+v", pf.Process)
	}
}

func TestFromSDK_NestedParamsRenest(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: mcp
        rules:
          - allow:
              params:
                arguments:
                  path: "/tmp/*"
`
	sdk, _ := Parse([]byte(y))
	pf := FromSDK(sdk)
	allow := pf.NetworkPolicies["p"].Endpoints[0].Rules[0].Allow
	args, ok := allow.Params["arguments"]
	if !ok || args.Object == nil {
		t.Fatalf("params not re-nested: %+v", allow.Params)
	}
	pathM, ok := args.Object["path"]
	if !ok || pathM.Matcher == nil || pathM.Matcher.Glob != "/tmp/*" {
		t.Errorf("nested path matcher wrong: %+v", args.Object)
	}
}

func TestSerialize_StructuralYAML(t *testing.T) {
	y := `
version: 1
network_policies:
  httpbin:
    name: httpbin
    endpoints:
      - host: httpbin.org
        port: 443
        protocol: rest
`
	sdk, _ := Parse([]byte(y))
	out, err := Serialize(sdk)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Structural content present; no document separator.
	if strings.Contains(s, "---") {
		t.Error("serialized policy should not contain ---")
	}
	for _, want := range []string{"version: 1", "httpbin", "host: httpbin.org", "protocol: rest"} {
		if !strings.Contains(s, want) {
			t.Errorf("serialized output missing %q:\n%s", want, s)
		}
	}
	// Re-parse to confirm it round-trips back through the loader.
	if _, err := Parse(out); err != nil {
		t.Errorf("serialized policy does not re-parse: %v", err)
	}
}

func TestSerialize_Nil(t *testing.T) {
	if out, err := Serialize(nil); out != nil || err != nil {
		t.Errorf("Serialize(nil) = %v, %v", out, err)
	}
}

func TestToJSONValue(t *testing.T) {
	sdk, _ := Parse([]byte("version: 1\n"))
	m := ToJSONValue(sdk)
	if m == nil {
		t.Fatal("ToJSONValue returned nil")
	}
	if v, ok := m["version"]; !ok || v.(float64) != 1 {
		t.Errorf("version in JSON view = %v", m["version"])
	}
	if ToJSONValue(nil) != nil {
		t.Error("ToJSONValue(nil) should be nil")
	}
}
