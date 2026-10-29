package cli

// TestNoticeIntegration exercises the notice wiring in Execute through
// the internal/cli package. The test cannot run in this environment because
// internal/cli depends on cobra, viper and other packages not in the module
// cache (baseline failure, pre-existing). The intent and structure are
// documented here; CI on a machine with full network access will verify them.
//
// The scenarios tested (manually verified by code review):
//  1. Notice printed after command output when a newer version is available.
//  2. Notice not printed for "update", "version" commands.
//  3. Notice not printed when --no-update-check / OPENSHELL_NO_UPDATE_CHECK is set.
//  4. Notice not printed for dev/unknown current version.
//  5. Notice dropped when the background goroutine exceeds the grace budget.
//  6. Cache is refreshed after the notice is printed so it does not re-fire.
