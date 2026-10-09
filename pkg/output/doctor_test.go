package output

import (
	"bytes"
	"strings"
	"testing"
)

func allGreenResults() []DoctorCheck {
	return []DoctorCheck{
		{Name: "Endpoint URL", Status: "pass", Detail: "https://gw.example.com (normalized: https://gw.example.com)"},
		{Name: "DNS", Status: "pass", Detail: "gw.example.com -> [10.0.0.1]"},
		{Name: "Credentials", Status: "pass", Detail: "minted token for subject \"svc\""},
	}
}

func mixedResults() []DoctorCheck {
	return []DoctorCheck{
		{Name: "Endpoint URL", Status: "pass", Detail: "https://gw.example.com"},
		{Name: "DNS", Status: "pass", Detail: "gw.example.com -> [10.0.0.1]"},
		{
			Name: "Credentials", Status: "fail",
			Detail:   "no credentials configured (checked: --token, OPENSHELL_OIDC_CLIENT_SECRET)",
			NextStep: "export OPENSHELL_OIDC_CLIENT_SECRET, or run `openshellctl login`",
		},
		{Name: "Audience", Status: "skip", Detail: "skipped: Credentials check failed"},
	}
}

func allSkippedResults() []DoctorCheck {
	return []DoctorCheck{
		{Name: "Audience", Status: "skip", Detail: "skipped: Credentials check failed"},
		{Name: "Roles", Status: "skip", Detail: "skipped: Credentials check failed"},
		{Name: "Providers", Status: "skip", Detail: "no --provider/-f given"},
	}
}

func TestRenderDoctor_TableAllGreen(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDoctor(&buf, allGreenResults(), FormatTable); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓", "Endpoint URL", "DNS", "Credentials", "svc"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "✗") || strings.Contains(got, "Hint:") {
		t.Errorf("all-green output should have no fail glyph or hint, got:\n%s", got)
	}
}

func TestRenderDoctor_TableMixed(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDoctor(&buf, mixedResults(), FormatTable); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓", "✗", "-", "Hint:", "openshellctl login"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderDoctor_TableAllSkipped(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDoctor(&buf, allSkippedResults(), FormatTable); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "✓") || strings.Contains(got, "✗") {
		t.Errorf("all-skipped output should have no pass/fail glyph, got:\n%s", got)
	}
	if !strings.Contains(got, "no --provider/-f given") {
		t.Errorf("output missing the not-applicable skip reason; got:\n%s", got)
	}
}

func TestRenderDoctor_TableEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDoctor(&buf, nil, FormatTable); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No checks") {
		t.Errorf("expected an empty-state message, got:\n%s", buf.String())
	}
}

func TestRenderDoctor_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDoctor(&buf, mixedResults(), FormatJSON); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(got), "[") || !strings.HasSuffix(strings.TrimSpace(got), "]") {
		t.Errorf("expected a JSON array, got:\n%s", got)
	}
	for _, want := range []string{`"name"`, `"status"`, `"detail"`, `"nextStep"`, `"fail"`, `"skip"`} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderDoctor_YAML(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDoctor(&buf, allGreenResults(), FormatYAML); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"name:", "status: pass", "detail:"} {
		if !strings.Contains(got, want) {
			t.Errorf("YAML missing %q; got:\n%s", want, got)
		}
	}
}
