package gatewayconfig

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func mapFSWith(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func TestErrorMessages(t *testing.T) {
	if got := (&GatewayNotFoundError{Name: "rosa"}).Error(); !strings.Contains(got, "rosa") {
		t.Errorf("GatewayNotFoundError = %q", got)
	}

	na := (&NoActiveGatewayError{}).Error()
	if !strings.HasPrefix(na, "No active gateway.\n") || !strings.Contains(na, "openshellctl gateway select <name>") {
		t.Errorf("NoActiveGatewayError verbatim mismatch:\n%s", na)
	}

	unk := (&UnknownGatewayError{Name: "ghost"}).Error()
	if !strings.HasPrefix(unk, "Unknown gateway 'ghost'.\n") ||
		!strings.Contains(unk, "--name ghost") ||
		!strings.Contains(unk, "openshellctl gateway select") {
		t.Errorf("UnknownGatewayError verbatim mismatch:\n%s", unk)
	}

	mp := &MetadataParseError{Name: "g", Cause: errors.New("bad json")}
	if !strings.Contains(mp.Error(), "bad json") {
		t.Errorf("MetadataParseError = %q", mp.Error())
	}
	if mp.Unwrap() == nil {
		t.Error("MetadataParseError should unwrap")
	}

	ge := &GatewayExistsError{Name: "dup"}
	if !strings.Contains(ge.Error(), "dup") {
		t.Errorf("GatewayExistsError = %q", ge.Error())
	}
	if !errors.Is(ge, ErrGatewayExists) {
		t.Error("GatewayExistsError should match ErrGatewayExists")
	}

	edge := &EdgeGatewayUnsupportedError{Endpoint: "https://edge.example.com"}
	if !strings.Contains(edge.Error(), "https://edge.example.com") || !strings.Contains(edge.Error(), "--oidc-issuer") {
		t.Errorf("EdgeGatewayUnsupportedError = %q", edge.Error())
	}
	if !errors.Is(edge, ErrEdgeGatewayUnsupported) {
		t.Error("EdgeGatewayUnsupportedError should match ErrEdgeGatewayUnsupported")
	}

	mtls := &MTLSUnsupportedError{}
	if !strings.Contains(mtls.Error(), "mTLS") {
		t.Errorf("MTLSUnsupportedError = %q", mtls.Error())
	}
	if !errors.Is(mtls, ErrMTLSUnsupported) {
		t.Error("MTLSUnsupportedError should match ErrMTLSUnsupported")
	}

	ie := &InvalidEndpointError{Endpoint: "bad", Cause: errors.New("boom")}
	if !strings.Contains(ie.Error(), "bad") || !strings.Contains(ie.Error(), "boom") {
		t.Errorf("InvalidEndpointError = %q", ie.Error())
	}
	if !errors.Is(ie, ErrInvalidEndpoint) {
		t.Error("InvalidEndpointError should match ErrInvalidEndpoint")
	}
	if ie.Unwrap() == nil {
		t.Error("InvalidEndpointError should unwrap")
	}
}

func TestFindByEndpoint_ScansAllGateways(t *testing.T) {
	// No active gateway; the match must be found by scanning all gateways.
	env := Env{
		UserFS: mapFSWith(map[string]string{
			"gateways/a/metadata.json": md("a", "https://a"),
			"gateways/b/metadata.json": md("b", "https://target"),
		}),
	}
	name, ok, err := FindByEndpoint(env, "https://target/")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if name != "b" {
		t.Errorf("name = %q, want b", name)
	}
}

func TestFindByEndpoint_NoMatch(t *testing.T) {
	env := Env{UserFS: mapFSWith(map[string]string{
		"gateways/a/metadata.json": md("a", "https://a"),
	})}
	_, ok, err := FindByEndpoint(env, "https://none")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected no match")
	}
}
