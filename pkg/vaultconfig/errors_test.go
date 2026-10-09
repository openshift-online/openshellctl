package vaultconfig

import (
	"strings"
	"testing"
)

// errContains is a small test helper shared by this file's and other
// vaultconfig _test.go files' message-content assertions.
func errContains(err error, substrs ...string) bool {
	msg := err.Error()
	for _, s := range substrs {
		if !strings.Contains(msg, s) {
			return false
		}
	}
	return true
}

func TestErrForbidden_Message(t *testing.T) {
	err := &ErrForbidden{Mount: "osd-sre", Path: "rosa-agent"}
	if !errContains(err, "osd-sre", "rosa-agent") {
		t.Errorf("ErrForbidden.Error() = %q, missing mount/path", err.Error())
	}
}

func TestErrSecretNotFound_Message(t *testing.T) {
	err := &ErrSecretNotFound{Mount: "osd-sre", Path: "rosa-agent"}
	if !errContains(err, "osd-sre", "rosa-agent") {
		t.Errorf("ErrSecretNotFound.Error() = %q, missing mount/path", err.Error())
	}
}

func TestErrFieldNotString_Message(t *testing.T) {
	err := &ErrFieldNotString{Field: "hypershell-oidc-client-id"}
	if !errContains(err, "hypershell-oidc-client-id") {
		t.Errorf("ErrFieldNotString.Error() = %q, missing field name", err.Error())
	}
}

func TestErrVaultAddrNotSet_Message(t *testing.T) {
	err := &ErrVaultAddrNotSet{}
	if !errContains(err, "VAULT_ADDR") {
		t.Errorf("ErrVaultAddrNotSet.Error() = %q, missing VAULT_ADDR", err.Error())
	}
}
