package policyyaml

import (
	"strings"
	"testing"
)

func TestParse_UnknownFieldRejected(t *testing.T) {
	_, err := Parse([]byte("version: 1\nbogus: true\n"))
	if err == nil {
		t.Fatal("expected unknown-field rejection")
	}
	if !strings.Contains(err.Error(), "failed to parse sandbox policy YAML") {
		t.Errorf("err = %v, want wrapped parse error", err)
	}
}

func TestParse_PortOutOfRange(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: example.com
        port: 70000
        protocol: rest
`
	if _, err := Parse([]byte(y)); err == nil {
		t.Error("expected error for port > 65535 (u16 overflow)")
	}
}

func TestToSDK_NameDefaultsFromKey(t *testing.T) {
	y := `
version: 1
network_policies:
  httpbin:
    endpoints:
      - host: httpbin.org
        port: 443
        protocol: rest
`
	p, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	rule := p.NetworkPolicies["httpbin"]
	if rule.Name != "httpbin" {
		t.Errorf("name = %q, want defaulted from key", rule.Name)
	}
}

func TestToSDK_PortsNormalization(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        ports: [443, 8443]
        protocol: rest
`
	p, _ := Parse([]byte(y))
	e := p.NetworkPolicies["p"].Endpoints[0]
	if e.Port != 443 {
		t.Errorf("port = %d, want ports[0]=443", e.Port)
	}
	if len(e.Ports) != 2 || e.Ports[0] != 443 || e.Ports[1] != 8443 {
		t.Errorf("ports = %v", e.Ports)
	}
}

func TestToSDK_SinglePortPromoted(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: rest
`
	p, _ := Parse([]byte(y))
	e := p.NetworkPolicies["p"].Endpoints[0]
	if e.Port != 443 || len(e.Ports) != 1 || e.Ports[0] != 443 {
		t.Errorf("port=%d ports=%v, want both 443", e.Port, e.Ports)
	}
}

func TestToSDK_MCPBodyBytesPrecedence(t *testing.T) {
	// mcp stanza present with max_body_bytes 0 shadows json_rpc.
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: mcp
        json_rpc:
          max_body_bytes: 5000
        mcp:
          max_body_bytes: 0
          allow_all_known_mcp_methods: true
`
	p, _ := Parse([]byte(y))
	e := p.NetworkPolicies["p"].Endpoints[0]
	if e.JSONRPCMaxBodyBytes != 0 {
		t.Errorf("JSONRPCMaxBodyBytes = %d, want 0 (mcp precedence)", e.JSONRPCMaxBodyBytes)
	}
	if e.Mcp == nil || e.Mcp.AllowAllKnownMcpMethods == nil || !*e.Mcp.AllowAllKnownMcpMethods {
		t.Errorf("Mcp options not set: %+v", e.Mcp)
	}
}

func TestToSDK_MCPOptionsOnlyWhenSet(t *testing.T) {
	// mcp with only max_body_bytes → no McpOptions emitted, but body bytes feeds json_rpc.
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: mcp
        mcp:
          max_body_bytes: 1500
`
	p, _ := Parse([]byte(y))
	e := p.NetworkPolicies["p"].Endpoints[0]
	if e.Mcp != nil {
		t.Errorf("Mcp should be nil when only max_body_bytes set, got %+v", e.Mcp)
	}
	if e.JSONRPCMaxBodyBytes != 1500 {
		t.Errorf("JSONRPCMaxBodyBytes = %d, want 1500", e.JSONRPCMaxBodyBytes)
	}
}

func TestToSDK_ToolShorthand(t *testing.T) {
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
              method: tools/call
              tool: my-tool
`
	p, _ := Parse([]byte(y))
	allow := p.NetworkPolicies["p"].Endpoints[0].Rules[0].Allow
	m, ok := allow.Params["name"]
	if !ok {
		t.Fatalf("tool should become params[name]; params = %v", allow.Params)
	}
	if m.Glob != "my-tool" {
		t.Errorf("params[name].Glob = %q, want my-tool", m.Glob)
	}
}

func TestToSDK_ToolDoesNotOverrideExistingName(t *testing.T) {
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
              tool: from-tool
              params:
                name: from-params
`
	p, _ := Parse([]byte(y))
	allow := p.NetworkPolicies["p"].Endpoints[0].Rules[0].Allow
	if allow.Params["name"].Glob != "from-params" {
		t.Errorf("existing params.name should win over tool, got %q", allow.Params["name"].Glob)
	}
}

func TestToSDK_NestedParamsFlattened(t *testing.T) {
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
	p, _ := Parse([]byte(y))
	allow := p.NetworkPolicies["p"].Endpoints[0].Rules[0].Allow
	m, ok := allow.Params["arguments.path"]
	if !ok {
		t.Fatalf("nested params should dot-flatten; params = %v", allow.Params)
	}
	if m.Glob != "/tmp/*" {
		t.Errorf("arguments.path = %q", m.Glob)
	}
}

func TestToSDK_QueryAnyMatcher(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: rest
        rules:
          - allow:
              method: GET
              query:
                repo:
                  any: ["NVIDIA/*", "openai/*"]
`
	p, _ := Parse([]byte(y))
	allow := p.NetworkPolicies["p"].Endpoints[0].Rules[0].Allow
	m := allow.Query["repo"]
	if len(m.Any) != 2 || m.Any[0] != "NVIDIA/*" {
		t.Errorf("query.repo.any = %v", m.Any)
	}
}

func TestToSDK_HarnessDropped(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: rest
    binaries:
      - path: /usr/bin/curl
        harness: true
`
	p, _ := Parse([]byte(y))
	bins := p.NetworkPolicies["p"].Binaries
	if len(bins) != 1 || bins[0].Path != "/usr/bin/curl" {
		t.Errorf("binaries = %v", bins)
	}
	// harness is accepted on decode but has no representation in the SDK type.
}

func TestToSDK_ProviderCredentialedFalse(t *testing.T) {
	y := `
version: 1
network_policies:
  p:
    endpoints:
      - host: h
        port: 443
        protocol: rest
`
	p, _ := Parse([]byte(y))
	e := p.NetworkPolicies["p"].Endpoints[0]
	if e.ProviderCredentialed || e.AdvisorProposed {
		t.Error("provider_credentialed/advisor_proposed must be false")
	}
}

func TestParseInline(t *testing.T) {
	m := map[string]any{
		"version": 1,
		"network_policies": map[string]any{
			"p": map[string]any{
				"endpoints": []any{
					map[string]any{"host": "h", "port": 443, "protocol": "rest"},
				},
			},
		},
	}
	p, err := ParseInline(m)
	if err != nil {
		t.Fatalf("ParseInline: %v", err)
	}
	if p.Version != 1 {
		t.Errorf("version = %d", p.Version)
	}
}
