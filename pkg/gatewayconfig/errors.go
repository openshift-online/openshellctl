package gatewayconfig

import (
	"errors"
	"fmt"
)

// Sentinels for errors.Is matching. The concrete error types below carry
// context (the offending name, the parse cause) and match these via Is.
var (
	ErrInvalidGatewayName     = errors.New("invalid gateway name")
	ErrGatewayNotFound        = errors.New("gateway not found")
	ErrNoActiveGateway        = errors.New("no active gateway")
	ErrUnknownGateway         = errors.New("unknown gateway")
	ErrMetadataParse          = errors.New("metadata parse error")
	ErrGatewayExists          = errors.New("gateway already exists")
	ErrEdgeGatewayUnsupported = errors.New("edge gateway registration unsupported")
	ErrMTLSUnsupported        = errors.New("mTLS gateway registration unsupported")
	ErrInvalidEndpoint        = errors.New("invalid gateway endpoint")
)

// InvalidGatewayNameError reports a name that is not a single path component.
type InvalidGatewayNameError struct{ Name string }

func (e *InvalidGatewayNameError) Error() string {
	return fmt.Sprintf("invalid gateway name '%s': expected a single path component", e.Name)
}

// Is matches the ErrInvalidGatewayName sentinel.
func (e *InvalidGatewayNameError) Is(target error) bool { return target == ErrInvalidGatewayName }

// GatewayNotFoundError reports that no metadata.json was found for a name in
// either the user or system tree.
type GatewayNotFoundError struct{ Name string }

func (e *GatewayNotFoundError) Error() string {
	return fmt.Sprintf("gateway %q not found", e.Name)
}

// Is matches the ErrGatewayNotFound sentinel.
func (e *GatewayNotFoundError) Is(target error) bool { return target == ErrGatewayNotFound }

// NoActiveGatewayError carries the verbatim upstream remediation block
// (Appendix A.13), adapted to name `openshellctl` — its own `gateway
// select`/`gateway add` commands are native, not delegated to the upstream
// `openshell` binary (see ROSAENG-68827).
type NoActiveGatewayError struct{}

func (e *NoActiveGatewayError) Error() string {
	return "No active gateway.\n" +
		"Set one with: openshellctl gateway select <name>\n" +
		"Or register one with: openshellctl gateway add <endpoint>"
}

// Is matches the ErrNoActiveGateway sentinel.
func (e *NoActiveGatewayError) Is(target error) bool { return target == ErrNoActiveGateway }

// UnknownGatewayError carries the verbatim upstream remediation block, with the
// name interpolated in three places (Appendix A.13), adapted to name
// `openshellctl` for the same reason as NoActiveGatewayError above.
type UnknownGatewayError struct{ Name string }

func (e *UnknownGatewayError) Error() string {
	return fmt.Sprintf(
		"Unknown gateway '%s'.\n"+
			"Register it first: openshellctl gateway add <endpoint> --name %s\n"+
			"Or list available gateways: openshellctl gateway select",
		e.Name, e.Name)
}

// Is matches the ErrUnknownGateway sentinel.
func (e *UnknownGatewayError) Is(target error) bool { return target == ErrUnknownGateway }

// MetadataParseError wraps a JSON decode failure for a gateway's metadata.json.
type MetadataParseError struct {
	Name  string
	Cause error
}

func (e *MetadataParseError) Error() string {
	return fmt.Sprintf("failed to parse metadata.json for gateway %q: %v", e.Name, e.Cause)
}

// Is matches the ErrMetadataParse sentinel.
func (e *MetadataParseError) Is(target error) bool { return target == ErrMetadataParse }

// Unwrap returns the underlying JSON decode error.
func (e *MetadataParseError) Unwrap() error { return e.Cause }

// GatewayExistsError reports that WriteGateway was asked to create a gateway
// that already has a metadata.json.
type GatewayExistsError struct{ Name string }

func (e *GatewayExistsError) Error() string {
	return fmt.Sprintf("gateway %q already exists", e.Name)
}

// Is matches the ErrGatewayExists sentinel.
func (e *GatewayExistsError) Is(target error) bool { return target == ErrGatewayExists }

// EdgeGatewayUnsupportedError reports that NewMetadata could not determine an
// OIDC issuer for the endpoint (no --oidc-issuer override and no successful
// /auth/oidc-config discovery) — i.e. the gateway doesn't look OIDC-configured.
// openshellctl's `gateway add` only supports registering OIDC gateways.
//
// Cause, when non-nil, is the discovery probe's own error — a DNS failure,
// TLS error, or timeout, as opposed to a clean non-200 response. Threading it
// through means a typo'd hostname is told what actually went wrong, instead
// of getting the same "pass --oidc-issuer" remediation a real non-OIDC
// gateway gets.
type EdgeGatewayUnsupportedError struct {
	Endpoint string
	Cause    error
}

func (e *EdgeGatewayUnsupportedError) Error() string {
	msg := fmt.Sprintf(
		"gateway at %q does not appear to be OIDC-configured (no --oidc-issuer given and "+
			"/auth/oidc-config discovery did not succeed); non-OIDC (edge) gateway registration "+
			"is not supported — pass --oidc-issuer explicitly if this gateway is OIDC-configured",
		e.Endpoint)
	if e.Cause != nil {
		msg += fmt.Sprintf("; discovery failed: %v", e.Cause)
	}
	return msg
}

// Is matches the ErrEdgeGatewayUnsupported sentinel.
func (e *EdgeGatewayUnsupportedError) Is(target error) bool {
	return target == ErrEdgeGatewayUnsupported
}

// Unwrap returns the discovery probe's own error, if any.
func (e *EdgeGatewayUnsupportedError) Unwrap() error { return e.Cause }

// MTLSUnsupportedError reports that mTLS gateway registration was requested.
// Out of scope for this epic (ROSAENG-68825) — openshellctl's `gateway add`
// only registers OIDC gateways.
type MTLSUnsupportedError struct{}

func (e *MTLSUnsupportedError) Error() string {
	return "mTLS gateway registration is not supported by `gateway add`"
}

// Is matches the ErrMTLSUnsupported sentinel.
func (e *MTLSUnsupportedError) Is(target error) bool { return target == ErrMTLSUnsupported }

// InvalidEndpointError reports a gateway endpoint that could not be parsed as
// a URL (after scheme defaulting), or was empty.
type InvalidEndpointError struct {
	Endpoint string
	Cause    error
}

func (e *InvalidEndpointError) Error() string {
	return fmt.Sprintf("invalid gateway endpoint %q: %v", e.Endpoint, e.Cause)
}

// Is matches the ErrInvalidEndpoint sentinel.
func (e *InvalidEndpointError) Is(target error) bool { return target == ErrInvalidEndpoint }

// Unwrap returns the underlying URL parse error, if any.
func (e *InvalidEndpointError) Unwrap() error { return e.Cause }
