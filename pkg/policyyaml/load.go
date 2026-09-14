package policyyaml

import (
	"encoding/json"
	"fmt"
	"io/fs"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"sigs.k8s.io/yaml"
)

// Load resolves a policy path (cli path, else $OPENSHELL_SANDBOX_POLICY, else
// none) and parses it. Mirrors load_sandbox_policy (lib.rs:1073). Returns
// (nil, false, nil) when no policy is configured.
func Load(path string, env func(string) string, fsys fs.FS) (*types.SandboxPolicy, bool, error) {
	resolved := path
	if resolved == "" && env != nil {
		resolved = env("OPENSHELL_SANDBOX_POLICY")
	}
	if resolved == "" {
		return nil, false, nil
	}
	data, err := fs.ReadFile(fsys, resolved)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read sandbox policy from %s: %w", resolved, err)
	}
	p, err := Parse(data)
	if err != nil {
		return nil, false, err
	}
	return p, true, nil
}

// Parse strictly decodes policy YAML into PolicyFile, then converts to the SDK
// type via ToSDK. Mirrors parse_sandbox_policy (lib.rs:1026). The client never
// runs the server-side validate_sandbox_policy.
func Parse(yamlBytes []byte) (*types.SandboxPolicy, error) {
	pf, err := decodePolicyFile(yamlBytes)
	if err != nil {
		return nil, err
	}
	return ToSDK(pf)
}

// ParseInline converts a manifest spec.policy (already a map[string]any) to the
// SDK type by JSON-round-tripping into Parse.
func ParseInline(m map[string]any) (*types.SandboxPolicy, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sandbox policy YAML: %w", err)
	}
	return Parse(data)
}

// decodePolicyFile strictly decodes YAML→PolicyFile, rejecting unknown fields.
func decodePolicyFile(yamlBytes []byte) (*PolicyFile, error) {
	var pf PolicyFile
	if err := yaml.UnmarshalStrict(yamlBytes, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse sandbox policy YAML: %w", err)
	}
	return &pf, nil
}
