package v1alpha1

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// Decode reads a manifest with strict unknown-field rejection, then checks the
// apiVersion/kind envelope.
func Decode(r io.Reader) (*Sandbox, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var s Sandbox
	if err := yaml.UnmarshalStrict(data, &s); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if s.APIVersion != APIVersion {
		return nil, &WrongAPIVersionError{Got: s.APIVersion}
	}
	if s.Kind != KindSandbox {
		return nil, &WrongKindError{Got: s.Kind}
	}
	return &s, nil
}

// Sentinels for envelope errors.
var (
	ErrWrongAPIVersion = errors.New("wrong apiVersion")
	ErrWrongKind       = errors.New("wrong kind")
)

// WrongAPIVersionError reports an unexpected apiVersion.
type WrongAPIVersionError struct{ Got string }

func (e *WrongAPIVersionError) Error() string {
	return fmt.Sprintf("apiVersion %q is not %q", e.Got, APIVersion)
}

// Is matches the ErrWrongAPIVersion sentinel.
func (e *WrongAPIVersionError) Is(t error) bool { return t == ErrWrongAPIVersion }

// WrongKindError reports an unexpected kind.
type WrongKindError struct{ Got string }

func (e *WrongKindError) Error() string {
	return fmt.Sprintf("kind %q is not %q", e.Got, KindSandbox)
}

// Is matches the ErrWrongKind sentinel.
func (e *WrongKindError) Is(t error) bool { return t == ErrWrongKind }

// FieldError is a validation failure on a specific field path.
type FieldError struct {
	Path string
	Msg  string
}

func (e *FieldError) Error() string { return e.Path + ": " + e.Msg }

// envKeyRe matches a valid environment variable name.
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate returns all validation errors (empty when valid). It is pure and
// self-contained (no dependency on pkg/sandbox) so the types stay a leaf package;
// the authoritative CPU/memory/forward validators in pkg/sandbox run again at
// flag-merge time. Rules per spec §5.4.
func (s *Sandbox) Validate() []error {
	var errs []error
	add := func(path, msg string) { errs = append(errs, &FieldError{Path: path, Msg: msg}) }

	if s.APIVersion != APIVersion {
		add("apiVersion", fmt.Sprintf("must be %q", APIVersion))
	}
	if s.Kind != KindSandbox {
		add("kind", fmt.Sprintf("must be %q", KindSandbox))
	}

	ws := s.Metadata.Workspace
	if ws == "" {
		ws = "default"
	}
	if strings.TrimSpace(ws) == "" {
		add("metadata.workspace", "must not be empty")
	}

	for k := range s.Metadata.Labels {
		if k == "" {
			add("metadata.labels", "label keys must not be empty")
		}
	}

	for k := range s.Spec.Env {
		if !envKeyRe.MatchString(k) {
			add("spec.env", fmt.Sprintf("invalid env key %q: must match [A-Za-z_][A-Za-z0-9_]*", k))
		}
		if strings.HasPrefix(k, "OPENSHELL_") {
			add("spec.env", fmt.Sprintf("env key %q: keys starting with OPENSHELL_ are reserved", k))
		}
	}

	if s.Spec.Resources != nil {
		r := s.Spec.Resources
		if r.CPU != "" && strings.TrimSpace(r.CPU) == "" {
			add("spec.resources.cpu", "must not be blank")
		}
		if r.Memory != "" && strings.TrimSpace(r.Memory) == "" {
			add("spec.resources.memory", "must not be blank")
		}
		if r.GPU != nil && r.GPU.Count != nil && *r.GPU.Count == 0 {
			add("spec.resources.gpu.count", "must be greater than 0 when set")
		}
	}

	for driver, v := range s.Spec.DriverConfig {
		if _, ok := v.(map[string]any); !ok {
			add("spec.driverConfig", fmt.Sprintf("value for driver %q must be a JSON object", driver))
		}
	}

	if len(s.Spec.Policy) > 0 && s.Spec.PolicyFile != "" {
		add("spec.policy", "policy and policyFile are mutually exclusive")
	}

	switch s.Spec.ApprovalMode {
	case "", "manual", "auto":
	default:
		add("spec.approvalMode", `must be one of "", "manual", "auto"`)
	}

	for i, u := range s.Spec.Upload {
		if u.Local == "" {
			add(fmt.Sprintf("spec.upload[%d].local", i), "must not be empty")
		}
	}

	seen := map[string]bool{}
	for i, p := range s.Spec.ProviderRefs {
		if p.Name == "" {
			add(fmt.Sprintf("spec.providerRefs[%d].name", i), "must not be empty")
		}
		if seen[p.Name] {
			add(fmt.Sprintf("spec.providerRefs[%d].name", i), fmt.Sprintf("duplicate provider %q", p.Name))
		}
		seen[p.Name] = true
	}

	return errs
}
