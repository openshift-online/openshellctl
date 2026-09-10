package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/openshellctl/pkg/auth"
)

func TestWriteToken_Text(t *testing.T) {
	tok := &auth.Token{
		Subject:  "user-1",
		Issuer:   "https://i",
		Audience: []string{"openshell-cli"},
		Roles:    []string{"openshell-user"},
		Expiry:   time.Now().Add(time.Hour),
		IssuedAt: time.Now().Add(-time.Minute),
		Source:   auth.SourceClientCredentials,
	}
	var b bytes.Buffer
	if err := writeToken(&b, tok, "client_credentials (gateway=rosa)", "text"); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, want := range []string{"user-1", "openshell-cli", "openshell-user", "Age:", "Expiry:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWriteToken_UnknownExpiry(t *testing.T) {
	tok := &auth.Token{Subject: "s", Source: auth.SourceStatic}
	var b bytes.Buffer
	if err := writeToken(&b, tok, "static", "text"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Expiry:    unknown") {
		t.Errorf("expected unknown expiry, got:\n%s", b.String())
	}
}

func TestWriteToken_JSON(t *testing.T) {
	tok := &auth.Token{
		Subject: "s",
		Expiry:  time.Unix(2000000000, 0),
		Source:  auth.SourceDisk,
	}
	var b bytes.Buffer
	if err := writeToken(&b, tok, "disk", "json"); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(b.Bytes(), &parsed); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b.String())
	}
	if parsed["source"] != "disk" {
		t.Errorf("source = %v", parsed["source"])
	}
	if _, ok := parsed["expiry"]; !ok {
		t.Error("expected expiry key in JSON")
	}
}

func TestStubCommandReturnsNotImplemented(t *testing.T) {
	root := NewRootCommand()
	var b bytes.Buffer
	root.SetOut(&b)
	root.SetErr(&b)
	root.SetArgs([]string{"sandbox", "create"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected NotImplementedError from a stub command")
	}
	if exitCodeFor(err) != ExitError {
		t.Errorf("stub exit = %d, want %d", exitCodeFor(err), ExitError)
	}
}
