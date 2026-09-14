package v1alpha1

import (
	"errors"
	"strings"
	"testing"
)

func TestDecode_Valid(t *testing.T) {
	yaml := `
apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: sb-1
  workspace: default
spec:
  image: ghcr.io/x/y:latest
  env:
    FOO: bar
`
	s, err := Decode(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if s.Metadata.Name != "sb-1" || s.Spec.Image != "ghcr.io/x/y:latest" {
		t.Errorf("decoded = %+v", s)
	}
}

func TestDecode_UnknownFieldRejected(t *testing.T) {
	yaml := `
apiVersion: openshell.managed.openshift.io/v1alpha1
kind: Sandbox
metadata:
  name: sb-1
spec:
  bogusField: 1
`
	if _, err := Decode(strings.NewReader(yaml)); err == nil {
		t.Error("expected strict decode to reject unknown field")
	}
}

func TestDecode_WrongAPIVersion(t *testing.T) {
	yaml := "apiVersion: wrong/v1\nkind: Sandbox\nmetadata: {}\nspec: {}\n"
	_, err := Decode(strings.NewReader(yaml))
	if !errors.Is(err, ErrWrongAPIVersion) {
		t.Fatalf("err = %v, want ErrWrongAPIVersion", err)
	}
}

func TestDecode_WrongKind(t *testing.T) {
	yaml := "apiVersion: openshell.managed.openshift.io/v1alpha1\nkind: Widget\nmetadata: {}\nspec: {}\n"
	_, err := Decode(strings.NewReader(yaml))
	if !errors.Is(err, ErrWrongKind) {
		t.Fatalf("err = %v, want ErrWrongKind", err)
	}
}

func validSandbox() *Sandbox {
	return &Sandbox{
		TypeMeta: TypeMeta{APIVersion: APIVersion, Kind: KindSandbox},
		Metadata: ObjectMeta{Name: "sb", Workspace: "default"},
	}
}

func TestValidate_OK(t *testing.T) {
	if errs := validSandbox().Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidate_Rules(t *testing.T) {
	u32 := func(n uint32) *uint32 { return &n }
	tests := []struct {
		name    string
		mutate  func(*Sandbox)
		wantSub string
	}{
		{"bad env key", func(s *Sandbox) { s.Spec.Env = map[string]string{"1BAD": "x"} }, "spec.env"},
		{"reserved env key", func(s *Sandbox) { s.Spec.Env = map[string]string{"OPENSHELL_X": "x"} }, "spec.env"},
		{"empty label key", func(s *Sandbox) { s.Metadata.Labels = map[string]string{"": "x"} }, "metadata.labels"},
		{"gpu count zero", func(s *Sandbox) { s.Spec.Resources = &Resources{GPU: &GPU{Count: u32(0)}} }, "gpu.count"},
		{"driverConfig not object", func(s *Sandbox) { s.Spec.DriverConfig = map[string]any{"d": "scalar"} }, "spec.driverConfig"},
		{"policy and policyFile", func(s *Sandbox) {
			s.Spec.Policy = map[string]any{"version": 1}
			s.Spec.PolicyFile = "p.yaml"
		}, "mutually exclusive"},
		{"bad approvalMode", func(s *Sandbox) { s.Spec.ApprovalMode = "sometimes" }, "spec.approvalMode"},
		{"empty upload local", func(s *Sandbox) { s.Spec.Upload = []Upload{{Local: ""}} }, "spec.upload[0].local"},
		{"empty providerRef", func(s *Sandbox) { s.Spec.ProviderRefs = []ProviderRef{{Name: ""}} }, "providerRefs[0].name"},
		{"dup providerRef", func(s *Sandbox) {
			s.Spec.ProviderRefs = []ProviderRef{{Name: "p"}, {Name: "p"}}
		}, "duplicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSandbox()
			tt.mutate(s)
			errs := s.Validate()
			if len(errs) == 0 {
				t.Fatalf("expected an error containing %q", tt.wantSub)
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), tt.wantSub) {
					found = true
				}
			}
			if !found {
				t.Errorf("errors %v do not contain %q", errs, tt.wantSub)
			}
		})
	}
}

func TestDeepCopy_Independent(t *testing.T) {
	tru := true
	cnt := uint32(2)
	orig := &Sandbox{
		TypeMeta: TypeMeta{APIVersion: APIVersion, Kind: KindSandbox},
		Metadata: ObjectMeta{Name: "sb", Labels: map[string]string{"a": "1"}},
		Spec: SandboxSpec{
			Command:      []string{"bash"},
			Env:          map[string]string{"FOO": "bar"},
			TTY:          &tru,
			Resources:    &Resources{CPU: "1", GPU: &GPU{Count: &cnt}},
			DriverConfig: map[string]any{"d": map[string]any{"k": "v"}},
			ProviderRefs: []ProviderRef{{Name: "p"}},
			Upload:       []Upload{{Local: "f", GitIgnore: &tru}},
		},
	}
	cp := orig.DeepCopy()

	// Mutate the copy; original must be unaffected.
	cp.Metadata.Labels["a"] = "2"
	cp.Spec.Env["FOO"] = "changed"
	cp.Spec.Command[0] = "sh"
	*cp.Spec.TTY = false
	*cp.Spec.Resources.GPU.Count = 9
	cp.Spec.DriverConfig["d"].(map[string]any)["k"] = "w"
	cp.Spec.ProviderRefs[0].Name = "q"

	if orig.Metadata.Labels["a"] != "1" {
		t.Error("labels not deep-copied")
	}
	if orig.Spec.Env["FOO"] != "bar" {
		t.Error("env not deep-copied")
	}
	if orig.Spec.Command[0] != "bash" {
		t.Error("command not deep-copied")
	}
	if *orig.Spec.TTY != true {
		t.Error("TTY pointer not deep-copied")
	}
	if *orig.Spec.Resources.GPU.Count != 2 {
		t.Error("GPU count not deep-copied")
	}
	if orig.Spec.DriverConfig["d"].(map[string]any)["k"] != "v" {
		t.Error("driverConfig not deep-copied")
	}
	if orig.Spec.ProviderRefs[0].Name != "p" {
		t.Error("providerRefs not deep-copied")
	}
}

func TestDeepCopy_Nil(t *testing.T) {
	var s *Sandbox
	if s.DeepCopy() != nil {
		t.Error("nil.DeepCopy() should be nil")
	}
}
