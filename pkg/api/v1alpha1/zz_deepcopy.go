package v1alpha1

// Hand-written deep-copy helpers (no controller-gen dependency in this module;
// the operator can regenerate with its own tooling). Spec §5.4.

// DeepCopy returns a deep copy of the Sandbox.
func (s *Sandbox) DeepCopy() *Sandbox {
	if s == nil {
		return nil
	}
	out := &Sandbox{
		TypeMeta: s.TypeMeta,
		Metadata: ObjectMeta{
			Name:      s.Metadata.Name,
			Workspace: s.Metadata.Workspace,
			Labels:    copyStringMap(s.Metadata.Labels),
		},
	}
	out.Spec = s.Spec.deepCopy()
	return out
}

func (in SandboxSpec) deepCopy() SandboxSpec {
	out := SandboxSpec{
		Image:                in.Image,
		Command:              copyStringSlice(in.Command),
		Env:                  copyStringMap(in.Env),
		ApprovalMode:         in.ApprovalMode,
		NoCredentialWarnings: in.NoCredentialWarnings,
		Detach:               in.Detach,
		Forward:              in.Forward,
		PolicyFile:           in.PolicyFile,
	}
	if in.TTY != nil {
		v := *in.TTY
		out.TTY = &v
	}
	if in.AutoProviders != nil {
		v := *in.AutoProviders
		out.AutoProviders = &v
	}
	if in.Keep != nil {
		v := *in.Keep
		out.Keep = &v
	}
	if len(in.ProviderRefs) > 0 {
		out.ProviderRefs = make([]ProviderRef, len(in.ProviderRefs))
		copy(out.ProviderRefs, in.ProviderRefs)
	}
	if in.Resources != nil {
		out.Resources = in.Resources.deepCopy()
	}
	out.DriverConfig = copyAnyMap(in.DriverConfig)
	out.Policy = copyAnyMap(in.Policy)
	if in.SessionOpts != nil {
		so := *in.SessionOpts
		out.SessionOpts = &so
	}
	if len(in.Upload) > 0 {
		out.Upload = make([]Upload, len(in.Upload))
		for i, u := range in.Upload {
			cu := Upload{Local: u.Local, Dest: u.Dest}
			if u.GitIgnore != nil {
				v := *u.GitIgnore
				cu.GitIgnore = &v
			}
			out.Upload[i] = cu
		}
	}
	return out
}

func (in *Resources) deepCopy() *Resources {
	out := &Resources{CPU: in.CPU, Memory: in.Memory}
	if in.GPU != nil {
		g := &GPU{}
		if in.GPU.Count != nil {
			v := *in.GPU.Count
			g.Count = &v
		}
		out.GPU = g
	}
	return out
}

func copyStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// copyAnyMap performs a deep copy of a JSON-like map[string]any (maps, slices,
// scalars). Sufficient for DriverConfig/Policy which come from JSON/YAML decode.
func copyAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = copyAny(v)
	}
	return out
}

func copyAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return copyAnyMap(x)
	case []any:
		s := make([]any, len(x))
		for i, e := range x {
			s[i] = copyAny(e)
		}
		return s
	default:
		return x
	}
}
