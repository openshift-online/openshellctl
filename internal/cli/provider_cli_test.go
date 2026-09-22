package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/openshift-online/openshellctl/pkg/gateway"
)

func TestProviderConflictAttachMessage(t *testing.T) {
	conflictErr := &gateway.ConflictError{Message: "aborted"}
	msg := "failed to attach provider: sandbox was modified by another operation, please retry the command"
	var conflict *gateway.ConflictError
	if !errors.As(conflictErr, &conflict) {
		t.Fatal("should match ConflictError")
	}
	if !strings.Contains(msg, "sandbox was modified by another operation") {
		t.Errorf("attach conflict message wrong: %q", msg)
	}
	if !strings.Contains(msg, "please retry the command") {
		t.Errorf("attach conflict message missing retry hint: %q", msg)
	}
}

func TestProviderConflictDetachMessage(t *testing.T) {
	msg := "failed to detach provider: sandbox was modified by another operation, please retry the command"
	if !strings.Contains(msg, "detach provider") {
		t.Errorf("detach conflict message wrong: %q", msg)
	}
}

func TestProviderConflictErrorMapsToExitConflict(t *testing.T) {
	err := &gateway.ConflictError{Message: "aborted"}
	if code := exitCodeFor(err); code != ExitConflict {
		t.Errorf("ConflictError exit code = %d, want %d", code, ExitConflict)
	}
}
