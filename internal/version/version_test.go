package version

import (
	"runtime/debug"
	"testing"
)

func TestGet_UsesInjectedValues(t *testing.T) {
	old := Version
	oldC := Commit
	oldP := OpenShellPin
	t.Cleanup(func() { Version, Commit, OpenShellPin = old, oldC, oldP })

	Version = "v1.2.3"
	Commit = "abc123"
	OpenShellPin = "v0.0.116"

	got := Get()
	if got.Version != "v1.2.3" {
		t.Errorf("Version = %q, want v1.2.3", got.Version)
	}
	if got.Commit != "abc123" {
		t.Errorf("Commit = %q, want abc123", got.Commit)
	}
	if got.OpenShellPin != "v0.0.116" {
		t.Errorf("OpenShellPin = %q, want v0.0.116", got.OpenShellPin)
	}
}

func TestSDKVersion_FoundInDeps(t *testing.T) {
	fake := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Deps: []*debug.Module{
			{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
			{Path: sdkModulePath, Version: "v0.0.0-20260828082717-d1155aa70042"},
		}}, true
	}
	if got := sdkVersion(fake); got != "v0.0.0-20260828082717-d1155aa70042" {
		t.Errorf("sdkVersion = %q, want the SDK pseudo-version", got)
	}
}

func TestSDKVersion_NoBuildInfo(t *testing.T) {
	fake := func() (*debug.BuildInfo, bool) { return nil, false }
	if got := sdkVersion(fake); got != "unknown" {
		t.Errorf("sdkVersion = %q, want unknown", got)
	}
}

func TestSDKVersion_DepAbsent(t *testing.T) {
	fake := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Deps: []*debug.Module{
			{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
		}}, true
	}
	if got := sdkVersion(fake); got != "unknown" {
		t.Errorf("sdkVersion = %q, want unknown", got)
	}
}
