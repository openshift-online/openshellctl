package sandbox

import (
	"fmt"
	"strconv"
	"strings"
)

// ForwardSpec is a parsed [bind:]port forward specification.
type ForwardSpec struct {
	Bind string
	Port uint16
}

// ParseForwardSpec parses "[bind_address:]port" (forward.rs:533-581). rfind ':';
// if the suffix parses as a u16 it is the port with the prefix as bind; else the
// whole string must be a u16. Port 0 is rejected. Default bind is 127.0.0.1.
func ParseForwardSpec(s string) (ForwardSpec, error) {
	invalid := fmt.Errorf("invalid forward spec '%s': expected [bind_address:]port", s)

	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		bind := s[:i]
		portStr := s[i+1:]
		port, err := parsePort(portStr)
		if err != nil {
			return ForwardSpec{}, invalid
		}
		if port == 0 {
			return ForwardSpec{}, fmt.Errorf("port must be between 1 and 65535")
		}
		if bind == "" {
			bind = "127.0.0.1"
		}
		return ForwardSpec{Bind: bind, Port: port}, nil
	}

	port, err := parsePort(s)
	if err != nil {
		return ForwardSpec{}, invalid
	}
	if port == 0 {
		return ForwardSpec{}, fmt.Errorf("port must be between 1 and 65535")
	}
	return ForwardSpec{Bind: "127.0.0.1", Port: port}, nil
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(n), nil
}

// AccessURL renders the access URL for a forward, showing 0.0.0.0/:: as localhost.
func (f ForwardSpec) AccessURL() string {
	host := f.Bind
	if host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d/", host, f.Port)
}
