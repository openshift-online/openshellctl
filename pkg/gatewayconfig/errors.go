package gatewayconfig

import (
	"errors"
	"fmt"
)

// Sentinels for errors.Is matching. The concrete error types below carry
// context (the offending name, the parse cause) and match these via Is.
var (
	ErrInvalidGatewayName = errors.New("invalid gateway name")
	ErrGatewayNotFound    = errors.New("gateway not found")
	ErrNoActiveGateway    = errors.New("no active gateway")
	ErrUnknownGateway     = errors.New("unknown gateway")
	ErrMetadataParse      = errors.New("metadata parse error")
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
// (Appendix A.13).
type NoActiveGatewayError struct{}

func (e *NoActiveGatewayError) Error() string {
	return "No active gateway.\n" +
		"Set one with: openshell gateway select <name>\n" +
		"Or register one with: openshell gateway add <endpoint>"
}

// Is matches the ErrNoActiveGateway sentinel.
func (e *NoActiveGatewayError) Is(target error) bool { return target == ErrNoActiveGateway }

// UnknownGatewayError carries the verbatim upstream remediation block, with the
// name interpolated in three places (Appendix A.13).
type UnknownGatewayError struct{ Name string }

func (e *UnknownGatewayError) Error() string {
	return fmt.Sprintf(
		"Unknown gateway '%s'.\n"+
			"Register it first: openshell gateway add <endpoint> --name %s\n"+
			"Or list available gateways: openshell gateway select",
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
