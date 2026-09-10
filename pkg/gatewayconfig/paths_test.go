package gatewayconfig

import (
	"errors"
	"testing"
)

func TestUserConfigDir(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		want   string
		errStr string
	}{
		{
			name: "XDG set (used verbatim)",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg"},
			want: "/xdg/openshell",
		},
		{
			// Divergence from upstream (documented): a getenv func cannot
			// distinguish unset from empty, so an empty XDG_CONFIG_HOME falls
			// back to HOME rather than yielding a relative "openshell" path.
			name: "XDG empty string falls back to HOME",
			env:  map[string]string{"XDG_CONFIG_HOME": "", "HOME": "/home/u"},
			want: "/home/u/.config/openshell",
		},
		{
			name: "falls back to HOME/.config when XDG unset",
			env:  map[string]string{"HOME": "/home/u"},
			want: "/home/u/.config/openshell",
		},
		{
			name:   "error when neither set",
			env:    map[string]string{},
			errStr: "HOME is not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			got, err := UserConfigDir(getenv)
			if tt.errStr != "" {
				if err == nil || err.Error() != tt.errStr {
					t.Fatalf("err = %v, want %q", err, tt.errStr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tt.want {
				t.Errorf("UserConfigDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSystemBaseDir(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"default", map[string]string{}, "/etc/openshell"},
		{"absolute override", map[string]string{"OPENSHELL_SYSTEM_GATEWAY_DIR": "/opt/os"}, "/opt/os"},
		{"empty override ignored", map[string]string{"OPENSHELL_SYSTEM_GATEWAY_DIR": ""}, "/etc/openshell"},
		{"relative override ignored", map[string]string{"OPENSHELL_SYSTEM_GATEWAY_DIR": "rel/path"}, "/etc/openshell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			if got := SystemBaseDir(getenv); got != tt.want {
				t.Errorf("SystemBaseDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateGatewayName(t *testing.T) {
	valid := []string{"rosa", "my-gateway", "gw_1", "a.b", "CI"}
	for _, n := range valid {
		if err := ValidateGatewayName(n); err != nil {
			t.Errorf("ValidateGatewayName(%q) = %v, want nil", n, err)
		}
	}

	invalid := []string{"", ".", "..", "a/b", "a/", "/a", "foo/../bar", "./x"}
	for _, n := range invalid {
		err := ValidateGatewayName(n)
		if err == nil {
			t.Errorf("ValidateGatewayName(%q) = nil, want error", n)
			continue
		}
		if !errors.Is(err, ErrInvalidGatewayName) {
			t.Errorf("ValidateGatewayName(%q) err not ErrInvalidGatewayName: %v", n, err)
		}
	}
}

func TestValidateGatewayName_Message(t *testing.T) {
	err := ValidateGatewayName("a/b")
	want := "invalid gateway name 'a/b': expected a single path component"
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

func TestGatewayDir(t *testing.T) {
	if got := GatewayDir("rosa"); got != "gateways/rosa" {
		t.Errorf("GatewayDir = %q, want gateways/rosa", got)
	}
}
