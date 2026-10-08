package gatewayconfig

import (
	"strings"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"bare https, no change needed", "https://host", "https://host", false},
		{"single trailing slash trimmed", "https://host/", "https://host", false},
		{"multiple trailing slashes trimmed", "https://host///", "https://host", false},
		{"explicit default https port stripped", "https://host:443", "https://host", false},
		{"explicit default https port + trailing slash", "https://host:443/", "https://host", false},
		{"explicit non-default port kept", "https://host:8443", "https://host:8443", false},
		{"explicit default http port stripped", "http://host:80", "http://host", false},
		{"explicit non-default http port kept", "http://host:8080", "http://host:8080", false},
		{"scheme case-insensitive", "HTTPS://host", "https://host", false},
		{"host case-insensitive", "https://HOST", "https://host", false},
		{"scheme and host both upper, with default port", "HTTPS://HOST:443/", "https://host", false},
		{"missing scheme defaults to https, matching EnsureScheme", "host:443", "https://host", false},
		{"missing scheme, no port", "host", "https://host", false},
		{"path is preserved (not stripped) beyond a bare trailing slash", "https://host/foo", "https://host/foo", false},
		{"trailing slash trimmed even with a path", "https://host/foo/", "https://host/foo", false},
		{"different paths must not collapse to the same value", "https://host/bar", "https://host/bar", false},
		{"query string preserved (a real distinguishing part of the URL)", "https://host?a=1", "https://host?a=1", false},
		{"fragment preserved", "https://host#frag", "https://host#frag", false},
		{"IPv6 literal, brackets preserved, default port stripped", "https://[::1]:443/", "https://[::1]", false},
		{"IPv6 literal, non-default port kept", "https://[::1]:8443/", "https://[::1]:8443", false},
		{"IPv6 literal, case folded", "https://[2001:DB8::1]:443/", "https://[2001:db8::1]", false},
		{"empty endpoint is an error", "", "", true},
		{"unparseable port is an error (EnsureScheme would not catch this)", "https://host:notaport", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeEndpoint(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeEndpoint(%q) = %q, nil; want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeEndpoint(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeEndpoint(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeEndpoint_EquivalenceMatrix confirms the actual bug this
// story fixes: the specific pairs the onboarding thread hit (trailing slash
// vs explicit default port) normalize identically, while a genuinely
// different port does not.
func TestNormalizeEndpoint_EquivalenceMatrix(t *testing.T) {
	equivalent := [][2]string{
		{"https://gw.example.com/", "https://gw.example.com:443"},
		{"https://gw.example.com", "https://gw.example.com:443/"},
		{"HTTPS://GW.example.com:443/", "https://gw.example.com"},
	}
	for _, pair := range equivalent {
		a, errA := NormalizeEndpoint(pair[0])
		b, errB := NormalizeEndpoint(pair[1])
		if errA != nil || errB != nil {
			t.Fatalf("unexpected error normalizing %v: %v / %v", pair, errA, errB)
		}
		if a != b {
			t.Errorf("expected %q and %q to normalize equal, got %q and %q", pair[0], pair[1], a, b)
		}
	}

	distinct := [][2]string{
		{"https://gw.example.com:443", "https://gw.example.com:8443"},
		{"https://gw.example.com/foo", "https://gw.example.com/bar"},
	}
	for _, pair := range distinct {
		a, errA := NormalizeEndpoint(pair[0])
		b, errB := NormalizeEndpoint(pair[1])
		if errA != nil || errB != nil {
			t.Fatalf("unexpected error normalizing %v: %v / %v", pair, errA, errB)
		}
		if a == b {
			t.Errorf("expected %q and %q to normalize distinctly, both got %q", pair[0], pair[1], a)
		}
	}
}

func TestNormalizeEndpoint_ErrorMentionsEndpoint(t *testing.T) {
	_, err := NormalizeEndpoint("https://host:notaport")
	if err == nil || !strings.Contains(err.Error(), "host:notaport") {
		t.Errorf("err = %v, want it to mention the offending endpoint", err)
	}
}
