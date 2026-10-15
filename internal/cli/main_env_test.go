package cli

import (
	"reflect"
	"testing"
)

// envNamesToScrub returns the sorted list of keys from environ (os.Environ()
// shaped "KEY=VALUE" strings) that start with any of prefixes. It is pure
// test scaffolding for TestMain's env scrub (see main_test.go) and carries no
// I/O of its own.
func envNamesToScrub(environ []string, prefixes []string) []string {
	var names []string
	for _, kv := range environ {
		key := kv
		for i, c := range kv {
			if c == '=' {
				key = kv[:i]
				break
			}
		}
		for _, p := range prefixes {
			if len(key) >= len(p) && key[:len(p)] == p {
				names = append(names, key)
				break
			}
		}
	}
	// Insertion sort keeps this dependency-free; the lists involved are tiny.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}

func TestEnvNamesToScrub(t *testing.T) {
	tests := []struct {
		name     string
		environ  []string
		prefixes []string
		want     []string
	}{
		{
			name:     "empty environ",
			environ:  nil,
			prefixes: []string{"OPENSHELL_", "VAULT_"},
			want:     nil,
		},
		{
			name:     "no matches",
			environ:  []string{"HOME=/root", "PATH=/usr/bin"},
			prefixes: []string{"OPENSHELL_", "VAULT_"},
			want:     nil,
		},
		{
			name: "mixed prefixes",
			environ: []string{
				"OPENSHELL_TOKEN=abc",
				"VAULT_ADDR=https://vault.example.com",
				"HOME=/root",
				"OPENSHELL_GATEWAY=rosa",
				"VAULT_TOKEN=s3cr3t",
			},
			prefixes: []string{"OPENSHELL_", "VAULT_"},
			want:     []string{"OPENSHELL_GATEWAY", "OPENSHELL_TOKEN", "VAULT_ADDR", "VAULT_TOKEN"},
		},
		{
			name:     "key with = in the value",
			environ:  []string{"OPENSHELL_TOKEN=a=b=c"},
			prefixes: []string{"OPENSHELL_"},
			want:     []string{"OPENSHELL_TOKEN"},
		},
		{
			name:     "key equal to the bare prefix",
			environ:  []string{"OPENSHELL_=whatever"},
			prefixes: []string{"OPENSHELL_"},
			want:     []string{"OPENSHELL_"},
		},
		{
			name:     "case sensitivity: lowercase is not matched",
			environ:  []string{"openshell_x=1", "OPENSHELL_X=2"},
			prefixes: []string{"OPENSHELL_"},
			want:     []string{"OPENSHELL_X"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envNamesToScrub(tt.environ, tt.prefixes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("envNamesToScrub(%v, %v) = %v, want %v", tt.environ, tt.prefixes, got, tt.want)
			}
		})
	}
}
