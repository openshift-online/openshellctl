package vaultconfig

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/hashicorp/vault/api"
)

func TestClassifyVaultError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want any // nil, *ErrForbidden, *ErrSecretNotFound, or "passthrough"
	}{
		{"nil error stays nil", nil, nil},
		{
			name: "secret not found sentinel maps to ErrSecretNotFound",
			err:  api.ErrSecretNotFound,
			want: &ErrSecretNotFound{Mount: "osd-sre", Path: "rosa-agent"},
		},
		{
			name: "wrapped secret not found sentinel still maps",
			err:  wrapErr(api.ErrSecretNotFound),
			want: &ErrSecretNotFound{Mount: "osd-sre", Path: "rosa-agent"},
		},
		{
			name: "403 response error maps to ErrForbidden",
			err:  &api.ResponseError{StatusCode: http.StatusForbidden},
			want: &ErrForbidden{Mount: "osd-sre", Path: "rosa-agent"},
		},
		{
			name: "500 response error passes through unchanged",
			err:  &api.ResponseError{StatusCode: http.StatusInternalServerError},
			want: "passthrough",
		},
		{
			name: "generic error passes through unchanged",
			err:  errors.New("network unreachable"),
			want: "passthrough",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyVaultError(tt.err, "osd-sre", "rosa-agent")
			switch want := tt.want.(type) {
			case nil:
				if got != nil {
					t.Errorf("classifyVaultError() = %v, want nil", got)
				}
			case *ErrSecretNotFound:
				var sn *ErrSecretNotFound
				if !errors.As(got, &sn) || *sn != *want {
					t.Errorf("classifyVaultError() = %#v, want %#v", got, want)
				}
			case *ErrForbidden:
				var fb *ErrForbidden
				if !errors.As(got, &fb) || *fb != *want {
					t.Errorf("classifyVaultError() = %#v, want %#v", got, want)
				}
			case string: // "passthrough"
				if got != tt.err {
					t.Errorf("classifyVaultError() = %v, want original error %v unchanged", got, tt.err)
				}
			}
		})
	}
}

// wrapErr wraps err so a sentinel is only reachable via errors.Is/errors.As,
// not direct identity — exercising classifyVaultError's use of errors.Is
// rather than a plain ==.
func wrapErr(err error) error { return &wrappedErr{err: err} }

type wrappedErr struct{ err error }

func (w *wrappedErr) Error() string { return "wrapped: " + w.err.Error() }
func (w *wrappedErr) Unwrap() error { return w.err }

func TestNewAPIClient_FailsFastWhenVaultAddrUnset(t *testing.T) {
	_, err := NewAPIClient(func(string) string { return "" })
	var addrErr *ErrVaultAddrNotSet
	if !errors.As(err, &addrErr) {
		t.Fatalf("expected *ErrVaultAddrNotSet, got %T: %v", err, err)
	}
}

func TestNewAPIClient_BuildsClientWhenVaultAddrSet(t *testing.T) {
	getenv := func(key string) string {
		if key == "VAULT_ADDR" {
			return "https://vault.example.com"
		}
		return ""
	}
	client, err := NewAPIClient(getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected a non-nil client")
	}
}

func TestSecretData(t *testing.T) {
	tests := []struct {
		name   string
		secret *api.KVSecret
		want   map[string]any
	}{
		{
			name:   "populated data is returned unchanged",
			secret: &api.KVSecret{Data: map[string]any{"oidc-client-id": "abc"}},
			want:   map[string]any{"oidc-client-id": "abc"},
		},
		{
			// vault kv delete (not kv destroy) on the latest version: per the
			// SDK's own doc comment, KVv2.Get returns no error but a nil Data.
			name:   "soft-deleted secret (nil Data, no error) maps to ErrSecretNotFound",
			secret: &api.KVSecret{Data: nil},
		},
		{
			name:   "nil secret maps to ErrSecretNotFound",
			secret: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := secretData(tt.secret, "osd-sre", "rosa-agent")
			if tt.want == nil {
				var notFound *ErrSecretNotFound
				if !errors.As(err, &notFound) {
					t.Fatalf("expected *ErrSecretNotFound, got %T: %v", err, err)
				}
				if notFound.Mount != "osd-sre" || notFound.Path != "rosa-agent" {
					t.Errorf("ErrSecretNotFound = %+v, want Mount=osd-sre Path=rosa-agent", notFound)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretData() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
