package cli

import (
	"bytes"
	"context"
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

// noticeInputsForTest returns a CheckInputs that passes every gating condition
// in ShouldCheck: both TTYs true, no CI, known stable version, stale cache.
// Callers override fields to test individual gates.
func noticeInputsForTest(current selfupdate.Current) selfupdate.CheckInputs {
	return selfupdate.CheckInputs{
		Command:        "sandbox",
		StdoutIsTTY:    true,
		StderrIsTTY:    true,
		CI:             "",
		CurrentVersion: current,
		CacheAge:       selfupdate.NoticeCacheTTL + 1,
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
	in := noticeInputsForTest(current)
	deps := cliDeps{
		NoticeInputs: &in,
		NoticeFetcher: &staticFetcher{releases: []selfupdate.Release{
			{TagName: "v0.1.6"},
		}},
	}

	_, notice, _ := runCmdCapturingNotice(t, deps, "sandbox", "list")

	if !strings.Contains(notice, "v0.1.6") {
		t.Errorf("notice missing v0.1.6; got %q", notice)
	}
	if !strings.Contains(notice, "openshellctl update") {
		t.Errorf("notice missing \"openshellctl update\"; got %q", notice)
	}
}

func TestNotice_AbsentForExcludedCommands(t *testing.T) {
	excludedCommands := []struct {
		noticeCmd string
		runArgs   []string
	}{
		{"update", []string{"update", "--check"}},
		{"version", []string{"version"}},
	}

	for _, tc := range excludedCommands {
		t.Run(tc.noticeCmd, func(t *testing.T) {
			current := selfupdate.ParseCurrent("v0.1.5")
			in := noticeInputsForTest(current)
			in.Command = tc.noticeCmd // ShouldCheck returns false for this command

			deps := cliDeps{
				NoticeInputs: &in,
				NoticeFetcher: &staticFetcher{releases: []selfupdate.Release{
					{TagName: "v0.1.6"},
				}},
			}

			_, notice, _ := runCmdCapturingNotice(t, deps, tc.runArgs...)

			if notice != "" {
				t.Errorf("expected no notice for command %q, got %q", tc.noticeCmd, notice)
			}
		})
	}
}

func TestNotice_AbsentWithNoUpdateCheckFlag(t *testing.T) {
	current := selfupdate.ParseCurrent("v0.1.5")
	in := noticeInputsForTest(current)
	in.NoUpdateCheck = true

	deps := cliDeps{
		NoticeInputs: &in,
		NoticeFetcher: &staticFetcher{releases: []selfupdate.Release{
			{TagName: "v0.1.6"},
		}},
	}

	_, notice, _ := runCmdCapturingNotice(t, deps, "sandbox", "list")

	if notice != "" {
		t.Errorf("expected no notice with --no-update-check, got %q", notice)
	}
}

func TestNotice_AbsentForDevVersion(t *testing.T) {
	current := selfupdate.ParseCurrent("dev") // Unknown: true
	in := noticeInputsForTest(current)

	deps := cliDeps{
		NoticeInputs: &in,
		NoticeFetcher: &staticFetcher{releases: []selfupdate.Release{
			{TagName: "v0.1.6"},
		}},
	}

	_, notice, _ := runCmdCapturingNotice(t, deps, "sandbox", "list")

	if notice != "" {
		t.Errorf("expected no notice for dev/unknown version, got %q", notice)
	}
}

func TestNotice_DroppedWhenFetcherExceedsGracePeriod(t *testing.T) {
	current := selfupdate.ParseCurrent("v0.1.5")
	in := noticeInputsForTest(current)

	// A fetcher that blocks longer than the grace period so collectNotice times out.
	blocker := &blockingFetcher{block: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) })

	deps := cliDeps{
		NoticeInputs:  &in,
		NoticeFetcher: blocker,
	}

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
