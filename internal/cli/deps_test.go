package cli

import (
	"context"
	"testing"
)

func TestDepsFrom_Present(t *testing.T) {
	want := cliDeps{}
	ctx := withDeps(context.Background(), want)

	got, ok := depsFrom(ctx)
	if !ok {
		t.Fatal("depsFrom: ok = false, want true for a context produced by withDeps")
	}
	if got != want {
		t.Errorf("depsFrom: got %+v, want %+v", got, want)
	}
}

func TestDepsFrom_Absent(t *testing.T) {
	_, ok := depsFrom(context.Background())
	if ok {
		t.Error("depsFrom: ok = true, want false for a plain context.Background()")
	}
}

func TestDepsFrom_WrongType(t *testing.T) {
	// Store a value of an unrelated type under the same context key type used
	// by withDeps, to confirm depsFrom's type assertion fails closed (ok ==
	// false) rather than panicking.
	ctx := context.WithValue(context.Background(), depsCtxKey{}, "not a cliDeps")

	_, ok := depsFrom(ctx)
	if ok {
		t.Error("depsFrom: ok = true, want false when the stored value is not a cliDeps")
	}
}
