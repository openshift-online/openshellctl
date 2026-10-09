package vaultconfig

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveToken(t *testing.T) {
	tests := []struct {
		name     string
		getenv   func(string) string
		readFile func(string) ([]byte, error)
		homeDir  func() (string, error)
		want     string
		wantErr  bool
	}{
		{
			// readFile/homeDir panic if called, enforcing short-circuit on env hit.
			name:     "VAULT_TOKEN env wins and the token-helper file is never consulted",
			getenv:   func(string) string { return "env-token" },
			readFile: func(string) ([]byte, error) { panic("ReadFile must not be called") },
			homeDir:  func() (string, error) { panic("HomeDir must not be called") },
			want:     "env-token",
		},
		{
			name:     "falls back to ~/.vault-token when env is empty",
			getenv:   func(string) string { return "" },
			homeDir:  func() (string, error) { return "/home/user", nil },
			readFile: func(path string) ([]byte, error) { return []byte("file-token"), nil },
			want:     "file-token",
		},
		{
			name:     "token-helper file content is trimmed",
			getenv:   func(string) string { return "" },
			homeDir:  func() (string, error) { return "/home/user", nil },
			readFile: func(path string) ([]byte, error) { return []byte("  file-token\n"), nil },
			want:     "file-token",
		},
		{
			name:    "reads the token-helper file from the injected home directory",
			getenv:  func(string) string { return "" },
			homeDir: func() (string, error) { return "/home/user", nil },
			readFile: func(path string) ([]byte, error) {
				if path != "/home/user/.vault-token" {
					t.Errorf("ReadFile called with %q, want %q", path, "/home/user/.vault-token")
				}
				return []byte("file-token"), nil
			},
			want: "file-token",
		},
		{
			name:    "no env, no home directory -> ErrNoToken",
			getenv:  func(string) string { return "" },
			homeDir: func() (string, error) { return "", errors.New("no home") },
			readFile: func(string) ([]byte, error) {
				panic("ReadFile must not be called when HomeDir fails")
			},
			wantErr: true,
		},
		{
			name:     "no env, file missing -> ErrNoToken",
			getenv:   func(string) string { return "" },
			homeDir:  func() (string, error) { return "/home/user", nil },
			readFile: func(string) ([]byte, error) { return nil, errors.New("not found") },
			wantErr:  true,
		},
		{
			name:     "no env, file empty/whitespace-only -> ErrNoToken",
			getenv:   func(string) string { return "" },
			homeDir:  func() (string, error) { return "/home/user", nil },
			readFile: func(string) ([]byte, error) { return []byte("   \n"), nil },
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveToken(TokenEnv{
				Getenv:   tt.getenv,
				ReadFile: tt.readFile,
				HomeDir:  tt.homeDir,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got token %q", got)
				}
				var noToken *ErrNoToken
				if !errors.As(err, &noToken) {
					t.Errorf("expected *ErrNoToken, got %T: %v", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrNoToken_Message(t *testing.T) {
	err := &ErrNoToken{}
	msg := err.Error()
	for _, want := range []string{"VAULT_TOKEN", ".vault-token", "vault login"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrNoToken.Error() = %q, missing %q", msg, want)
		}
	}
}
