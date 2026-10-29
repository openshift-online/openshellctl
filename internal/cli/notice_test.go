package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/selfupdate"
)

// staticFetcher is a selfupdate.Fetcher that returns a fixed release list.
type staticFetcher struct {
	releases []selfupdate.Release
	listErr  error
}

func (f *staticFetcher) ListReleases(_ context.Context) ([]selfupdate.Release, error) {
	return f.releases, f.listErr
}

func (f *staticFetcher) GetRelease(_ context.Context, tag string) (selfupdate.Release, error) {
	for _, r := range f.releases {
		if r.TagName == tag {
			return r, nil
		}
	}
	return selfupdate.Release{}, &selfupdate.ErrTagNotFound{Tag: tag}
}

// noticeEnvForTest returns a CheckInputs whose env-derived fields pass every
// gating condition in ShouldCheck: both TTYs true, no CI, known stable version,
// stale cache. Tests override individual fields to exercise specific gates.
// Command and NoUpdateCheck are NOT set here — they always come from the real
// cobra invocation via startNoticeGoroutine.
func noticeEnvForTest(current selfupdate.Current) selfupdate.CheckInputs {
	return selfupdate.CheckInputs{
		StdoutIsTTY:    true,
		StderrIsTTY:    true,
		CI:             "",
		CurrentVersion: current,
		CacheAge:       selfupdate.NoticeCacheTTL + 1,
	}
}

// noticeDeps builds a cliDeps with the given env override and fetcher, pointing
// the cache at an empty temp directory so on-disk cache never interferes.
func noticeDeps(t *testing.T, override *selfupdate.CheckInputs, fetcher selfupdate.Fetcher) cliDeps {
	t.Helper()
	return cliDeps{
		NoticeEnvOverride: override,
		NoticeFetcher:     fetcher,
		NoticeCachePath:   filepath.Join(t.TempDir(), "update-check.json"),
	}
}

// runCmdCapturingNotice runs the command tree with injected deps, then
// collects the background notice separately from command output. Returns
// command stdout+stderr (combined), notice text, and any error from Execute.
func runCmdCapturingNotice(t *testing.T, deps cliDeps, args ...string) (string, string, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetVaultOnce()
	t.Cleanup(resetVaultOnce)

	root := NewRootCommand()
	root.SetContext(withDeps(context.Background(), deps))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	var notice bytes.Buffer
	collectNotice(root, &notice)
	return out.String(), notice.String(), err
}

func TestNotice_PrintedWhenNewerVersionAvailable(t *testing.T) {
	current := selfupdate.ParseCurrent("v0.1.5")
	env := noticeEnvForTest(current)
	deps := noticeDeps(t, &env, &staticFetcher{releases: []selfupdate.Release{
		{TagName: "v0.1.6"},
	}})

	_, notice, _ := runCmdCapturingNotice(t, deps, "sandbox", "list")

	if !strings.Contains(notice, "v0.1.6") {
		t.Errorf("notice missing v0.1.6; got %q", notice)
	}
	if !strings.Contains(notice, "openshellctl update") {
		t.Errorf("notice missing \"openshellctl update\"; got %q", notice)
	}
}

// TestNotice_AbsentForExcludedCommands verifies that no update notice is
// printed when the user invokes an excluded command. Command exclusion is
// gated by cmd.Name() in startNoticeGoroutine, which flows from the real
// cobra args — no hand-setting of Command in the override.
func TestNotice_AbsentForExcludedCommands(t *testing.T) {
	current := selfupdate.ParseCurrent("v0.1.5")
	env := noticeEnvForTest(current)
	deps := noticeDeps(t, &env, &staticFetcher{releases: []selfupdate.Release{
		{TagName: "v0.1.6"},
	}})

	// "version" has a RunE that exercises the full PersistentPreRunE path;
	// cmd.Name()="version" → ShouldCheck returns false → no notice.
	_, notice, _ := runCmdCapturingNotice(t, deps, "version")

	if notice != "" {
		t.Errorf("expected no notice for excluded command %q, got %q", "version", notice)
	}
}

// TestNotice_AbsentWithNoUpdateCheckFlag verifies that --no-update-check
// suppresses the notice. The flag is parsed by cobra and flows through
// pf.noUpdateCheck in startNoticeGoroutine — not set in the env override.
func TestNotice_AbsentWithNoUpdateCheckFlag(t *testing.T) {
	current := selfupdate.ParseCurrent("v0.1.5")
	env := noticeEnvForTest(current)
	deps := noticeDeps(t, &env, &staticFetcher{releases: []selfupdate.Release{
		{TagName: "v0.1.6"},
	}})

	_, notice, _ := runCmdCapturingNotice(t, deps, "--no-update-check", "sandbox", "list")

	if notice != "" {
		t.Errorf("expected no notice with --no-update-check, got %q", notice)
	}
}

func TestNotice_AbsentForDevVersion(t *testing.T) {
	current := selfupdate.ParseCurrent("dev") // Unknown: true
	env := noticeEnvForTest(current)
	deps := noticeDeps(t, &env, &staticFetcher{releases: []selfupdate.Release{
		{TagName: "v0.1.6"},
	}})

	_, notice, _ := runCmdCapturingNotice(t, deps, "sandbox", "list")

	if notice != "" {
		t.Errorf("expected no notice for dev/unknown version, got %q", notice)
	}
}

func TestNotice_DroppedWhenFetcherExceedsGracePeriod(t *testing.T) {
	current := selfupdate.ParseCurrent("v0.1.5")
	env := noticeEnvForTest(current)

	// A fetcher that blocks longer than the grace period so collectNotice times out.
	blocker := &blockingFetcher{block: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) })

	deps := noticeDeps(t, &env, blocker)

	start := time.Now()
	_, notice, _ := runCmdCapturingNotice(t, deps, "sandbox", "list")
	elapsed := time.Since(start)

	if notice != "" {
		t.Errorf("expected no notice when fetcher blocks, got %q", notice)
	}
	// collectNotice waits at most noticePrintGrace (250ms); verify we didn't hang.
	if elapsed > 2*time.Second {
		t.Errorf("runCmdCapturingNotice took %v, want < 2s", elapsed)
	}
}

// blockingFetcher is a Fetcher whose ListReleases blocks until the channel is
// closed, simulating a slow network call that exceeds the grace period.
type blockingFetcher struct{ block chan struct{} }

func (f *blockingFetcher) ListReleases(ctx context.Context) ([]selfupdate.Release, error) {
	select {
	case <-f.block:
	case <-ctx.Done():
	}
	return nil, ctx.Err()
}

func (f *blockingFetcher) GetRelease(_ context.Context, tag string) (selfupdate.Release, error) {
	return selfupdate.Release{}, &selfupdate.ErrTagNotFound{Tag: tag}
}
