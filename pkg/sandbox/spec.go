package sandbox

import (
	"encoding/json"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

// DefaultCommand is the sandbox default command when none is given.
var DefaultCommand = []string{"/bin/bash", "-l"}

// ToSDKSpec builds the SDK SandboxSpec from a resolved CreateRequest. It
// populates only the fields the CLI sets (§5.5): Environment, Providers, Policy,
// Command (defaulted), TTY, GPUCount, and Template (only when image/resources/
// driverConfig are set). ttyResolved is the auto-detected/overridden TTY value.
//
// Note: GPUCount is set only for an explicit count. A bare --gpu (GPU != nil &&
// GPU.Count == nil) cannot be expressed via the SDK spec (count: None) and must
// go through the raw CreateSandbox path — Create checks UsesRawGPU for this.
func ToSDKSpec(r *CreateRequest, ttyResolved bool) *types.SandboxSpec {
	spec := &types.SandboxSpec{
		Environment: r.Env,
		Providers:   r.Providers,
		Policy:      r.Policy,
		Command:     r.Command,
		TTY:         ttyResolved,
	}
	if len(spec.Command) == 0 {
		spec.Command = append([]string(nil), DefaultCommand...)
	}
	if r.GPU != nil && r.GPU.Count != nil {
		spec.GPUCount = r.GPU.Count
	}

	if r.Image != "" || r.CPU != "" || r.Memory != "" || len(r.DriverConfig) > 0 {
		tmpl := &types.SandboxTemplate{Image: r.Image}
		if res := ResourcesStruct(r.CPU, r.Memory); res != nil {
			tmpl.Resources = res
		}
		if len(r.DriverConfig) > 0 {
			tmpl.DriverConfig = coerceJSON(r.DriverConfig)
		}
		spec.Template = tmpl
	}

	return spec
}

// UsesRawGPU reports whether the request needs the raw CreateSandbox path
// (bare --gpu / gpu: {} — a GPU request with no explicit count).
func (r *CreateRequest) UsesRawGPU() bool {
	return r.GPU != nil && r.GPU.Count == nil
}

// coerceJSON round-trips a map through JSON so integers become float64 etc.,
// matching what structpb.NewStruct accepts.
func coerceJSON(m map[string]any) map[string]any {
	data, err := json.Marshal(m)
	if err != nil {
		return m
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return m
	}
	return out
}
