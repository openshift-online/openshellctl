// Package policyyaml is a Go port of crates/openshell-policy's YAML loader and
// serializer (spec §5.6, Appendix B). It parses the sandbox-policy YAML into the
// PolicyFile schema (strict, deny-unknown-fields), converts to the SDK's
// types.SandboxPolicy via to_proto semantics, and serializes back. The client
// never runs the server-side validate_sandbox_policy; that is exposed separately
// as Lint for `openshellctl policy lint`.
package policyyaml

// PolicyFile is the top-level YAML schema (lib.rs:55). version is required; all
// other fields default. Every struct rejects unknown fields on decode.
type PolicyFile struct {
	Version            uint32                                `json:"version"`
	FilesystemPolicy   *FilesystemDef                        `json:"filesystem_policy,omitempty"`
	Landlock           *LandlockDef                          `json:"landlock,omitempty"`
	Process            *ProcessDef                           `json:"process,omitempty"`
	NetworkPolicies    map[string]NetworkPolicyRuleDef       `json:"network_policies,omitempty"`
	NetworkMiddlewares map[string]NetworkMiddlewareConfigDef `json:"network_middlewares,omitempty"`
}

// FilesystemDef mirrors FilesystemDef (lib.rs:71). include_workdir is always
// emitted on serialize.
type FilesystemDef struct {
	IncludeWorkdir bool     `json:"include_workdir"`
	ReadOnly       []string `json:"read_only,omitempty"`
	ReadWrite      []string `json:"read_write,omitempty"`
}

// LandlockDef mirrors LandlockDef. compatibility "" | best_effort | hard_requirement.
type LandlockDef struct {
	Compatibility string `json:"compatibility,omitempty"`
}

// ProcessDef mirrors ProcessDef.
type ProcessDef struct {
	RunAsUser  string `json:"run_as_user,omitempty"`
	RunAsGroup string `json:"run_as_group,omitempty"`
}

// NetworkPolicyRuleDef mirrors NetworkPolicyRuleDef (lib.rs:98). name defaults
// from the map key.
type NetworkPolicyRuleDef struct {
	Name      string               `json:"name,omitempty"`
	Endpoints []NetworkEndpointDef `json:"endpoints,omitempty"`
	Binaries  []NetworkBinaryDef   `json:"binaries,omitempty"`
}

// NetworkEndpointDef mirrors NetworkEndpointDef (lib.rs:115-178).
type NetworkEndpointDef struct {
	Host  string   `json:"host,omitempty"`
	Path  string   `json:"path,omitempty"`
	Port  uint16   `json:"port,omitempty"` // mutually exclusive with ports
	Ports []uint16 `json:"ports,omitempty"`

	Protocol    string `json:"protocol,omitempty"`
	TLS         string `json:"tls,omitempty"`
	Enforcement string `json:"enforcement,omitempty"`
	Access      string `json:"access,omitempty"`

	Rules      []L7RuleDef     `json:"rules,omitempty"`
	AllowedIPs []string        `json:"allowed_ips,omitempty"`
	DenyRules  []L7DenyRuleDef `json:"deny_rules,omitempty"`

	AllowEncodedSlash            bool `json:"allow_encoded_slash,omitempty"`
	WebsocketCredentialRewrite   bool `json:"websocket_credential_rewrite,omitempty"`
	RequestBodyCredentialRewrite bool `json:"request_body_credential_rewrite,omitempty"`
	AllowUninspectedCredentials  bool `json:"allow_uninspected_credentials,omitempty"`

	PersistedQueries        string                         `json:"persisted_queries,omitempty"`
	GraphqlPersistedQueries map[string]GraphqlOperationDef `json:"graphql_persisted_queries,omitempty"`
	GraphqlMaxBodyBytes     uint32                         `json:"graphql_max_body_bytes,omitempty"`

	CredentialSigning string `json:"credential_signing,omitempty"`
	SigningService    string `json:"signing_service,omitempty"`
	SigningRegion     string `json:"signing_region,omitempty"`

	CredentialBinding *NetworkCredentialBindingDef `json:"credential_binding,omitempty"`
	JSONRPC           *JSONRPCConfigDef            `json:"json_rpc,omitempty"`
	MCP               *MCPConfigDef                `json:"mcp,omitempty"`
}

// NetworkCredentialBindingDef mirrors NetworkCredentialBindingDef. provider required.
type NetworkCredentialBindingDef struct {
	Provider string `json:"provider"`
}

// JSONRPCConfigDef mirrors JsonRpcConfigDef.
type JSONRPCConfigDef struct {
	MaxBodyBytes uint32 `json:"max_body_bytes,omitempty"`
}

// MCPConfigDef mirrors McpConfigDef.
type MCPConfigDef struct {
	MaxBodyBytes            uint32 `json:"max_body_bytes,omitempty"`
	StrictToolNames         *bool  `json:"strict_tool_names,omitempty"`
	AllowAllKnownMcpMethods *bool  `json:"allow_all_known_mcp_methods,omitempty"`
}

// GraphqlOperationDef mirrors GraphqlOperationDef.
type GraphqlOperationDef struct {
	OperationType string   `json:"operation_type,omitempty"`
	OperationName string   `json:"operation_name,omitempty"`
	Fields        []string `json:"fields,omitempty"`
}

// L7RuleDef wraps an allow clause (required key "allow").
type L7RuleDef struct {
	Allow L7AllowDef `json:"allow"`
}

// L7AllowDef mirrors L7AllowDef.
type L7AllowDef struct {
	Method        string                  `json:"method,omitempty"`
	Path          string                  `json:"path,omitempty"`
	Command       string                  `json:"command,omitempty"`
	Query         map[string]QueryMatcher `json:"query,omitempty"`
	OperationType string                  `json:"operation_type,omitempty"`
	OperationName string                  `json:"operation_name,omitempty"`
	Fields        []string                `json:"fields,omitempty"`
	Tool          *QueryMatcher           `json:"tool,omitempty"`
	Params        map[string]ParamMatcher `json:"params,omitempty"`
}

// L7DenyRuleDef is the same field set as L7AllowDef with no "allow" wrapper.
type L7DenyRuleDef struct {
	Method        string                  `json:"method,omitempty"`
	Path          string                  `json:"path,omitempty"`
	Command       string                  `json:"command,omitempty"`
	Query         map[string]QueryMatcher `json:"query,omitempty"`
	OperationType string                  `json:"operation_type,omitempty"`
	OperationName string                  `json:"operation_name,omitempty"`
	Fields        []string                `json:"fields,omitempty"`
	Tool          *QueryMatcher           `json:"tool,omitempty"`
	Params        map[string]ParamMatcher `json:"params,omitempty"`
}

// NetworkBinaryDef mirrors NetworkBinaryDef. path required; harness accepted,
// ignored, never emitted.
type NetworkBinaryDef struct {
	Path    string `json:"path"`
	Harness bool   `json:"harness,omitempty"`
}

// NetworkMiddlewareConfigDef mirrors middleware.rs NetworkMiddlewareConfigDef.
type NetworkMiddlewareConfigDef struct {
	Name       string                         `json:"name,omitempty"`
	Middleware string                         `json:"middleware"`
	Order      int32                          `json:"order,omitempty"`
	Config     map[string]any                 `json:"config,omitempty"`
	OnError    string                         `json:"on_error,omitempty"`
	Endpoints  *MiddlewareEndpointSelectorDef `json:"endpoints,omitempty"`
}

// MiddlewareEndpointSelectorDef mirrors MiddlewareEndpointSelectorDef.
type MiddlewareEndpointSelectorDef struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}
