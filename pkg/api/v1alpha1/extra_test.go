package v1alpha1

import (
	"strings"
	"testing"
)

func TestEnvelopeErrorMessages(t *testing.T) {
	if got := (&WrongAPIVersionError{Got: "x/v1"}).Error(); !strings.Contains(got, "x/v1") || !strings.Contains(got, APIVersion) {
		t.Errorf("WrongAPIVersionError = %q", got)
	}
	if got := (&WrongKindError{Got: "Widget"}).Error(); !strings.Contains(got, "Widget") || !strings.Contains(got, KindSandbox) {
		t.Errorf("WrongKindError = %q", got)
	}
	if got := (&FieldError{Path: "spec.x", Msg: "bad"}).Error(); got != "spec.x: bad" {
		t.Errorf("FieldError = %q", got)
	}
}

func TestDeepCopy_NestedSlicesAndNilResources(t *testing.T) {
	orig := &Sandbox{
		Spec: SandboxSpec{
			DriverConfig: map[string]any{
				"d": map[string]any{"list": []any{"a", "b", map[string]any{"n": 1}}},
			},
		},
	}
	cp := orig.DeepCopy()
	// Mutating the copy's nested slice must not touch the original.
	inner := cp.Spec.DriverConfig["d"].(map[string]any)["list"].([]any)
	inner[0] = "changed"
	origList := orig.Spec.DriverConfig["d"].(map[string]any)["list"].([]any)
	if origList[0] != "a" {
		t.Error("nested slice not deep-copied")
	}
	// nil Resources stays nil.
	if cp.Spec.Resources != nil {
		t.Error("nil Resources should stay nil")
	}
}
