package provider

import (
	"testing"
	"time"
)

func TestParseCredentialPairs(t *testing.T) {
	getenv := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	cases := []struct {
		name    string
		items   []string
		env     map[string]string
		want    map[string]string
		wantErr string
	}{
		{
			name:  "inline KEY=VALUE",
			items: []string{"FOO=bar"},
			want:  map[string]string{"FOO": "bar"},
		},
		{
			name:  "value may itself contain '='",
			items: []string{"FOO=bar=baz"},
			want:  map[string]string{"FOO": "bar=baz"},
		},
		{
			name:  "bare KEY looks up env",
			items: []string{"FOO"},
			env:   map[string]string{"FOO": "from-env"},
			want:  map[string]string{"FOO": "from-env"},
		},
		{
			name:    "bare KEY with no env value errors",
			items:   []string{"FOO"},
			env:     map[string]string{},
			wantErr: "FOO",
		},
		{
			name:    "empty key before '=' errors",
			items:   []string{"=bar"},
			wantErr: "empty",
		},
		{
			name:  "no items returns empty map",
			items: nil,
			want:  map[string]string{},
		},
		{
			name:  "later duplicate key wins",
			items: []string{"FOO=1", "FOO=2"},
			want:  map[string]string{"FOO": "2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCredentialPairs(tc.items, getenv(tc.env))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !mapsEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseConfigPairs(t *testing.T) {
	cases := []struct {
		name    string
		items   []string
		want    map[string]string
		wantErr string
	}{
		{name: "inline KEY=VALUE", items: []string{"org=acme"}, want: map[string]string{"org": "acme"}},
		{name: "no items returns empty map", items: nil, want: map[string]string{}},
		{name: "missing '=' errors", items: []string{"org"}, wantErr: "KEY=VALUE"},
		{name: "empty key errors", items: []string{"=acme"}, wantErr: "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseConfigPairs(tc.items)
			if tc.wantErr != "" {
				if err == nil || !contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !mapsEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseCredentialExpiresAt(t *testing.T) {
	cases := []struct {
		name    string
		items   []string
		want    map[string]time.Time
		wantErr string
	}{
		{
			name:  "epoch milliseconds",
			items: []string{"FOO=1700000000000"},
			want:  map[string]time.Time{"FOO": time.UnixMilli(1700000000000).UTC()},
		},
		{
			name:  "RFC3339",
			items: []string{"FOO=2024-01-01T00:00:00Z"},
			want:  map[string]time.Time{"FOO": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:  "zero clears expiry",
			items: []string{"FOO=0"},
			want:  map[string]time.Time{"FOO": {}},
		},
		{
			name:    "missing '=' errors",
			items:   []string{"FOO"},
			wantErr: "KEY=TIMESTAMP",
		},
		{
			name:    "empty key errors",
			items:   []string{"=1700000000000"},
			wantErr: "empty",
		},
		{
			name:    "unparseable timestamp errors",
			items:   []string{"FOO=not-a-timestamp"},
			wantErr: "FOO",
		},
		{
			name:  "no items returns empty map",
			items: nil,
			want:  map[string]time.Time{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCredentialExpiresAt(tc.items)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for k, v := range tc.want {
				gv, ok := got[k]
				if !ok {
					t.Fatalf("missing key %q in %+v", k, got)
				}
				if !gv.Equal(v) {
					t.Errorf("key %q: got %v, want %v", k, gv, v)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (substr == "" || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
