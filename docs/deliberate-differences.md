# Deliberate differences from upstream

This document records places where openshellctl's behaviour intentionally
diverges from the upstream Rust CLI (`openshell`) or from `ssh`. Each entry
states the difference, the reason, and where in the code it is implemented.

## Terminal mode reset after interactive attach

**Difference**: When `sandbox create` or `sandbox connect` ends an interactive
session (exit or Ctrl-P Ctrl-Q detach), openshellctl writes ANSI/VT100 escape
sequences to the local terminal before returning. The Rust CLI and `ssh` do
not send these sequences.

**Sequences sent** (each idempotent on VTE/xterm-family terminals):

| Sequence | Effect |
|---|---|
| `\033[?1000l` | Disable X10 mouse button events |
| `\033[?1002l` | Disable button-event mouse tracking |
| `\033[?1003l` | Disable any-event mouse tracking |
| `\033[?1006l` | Disable SGR extended mouse encoding |
| `\033[?1004l` | Disable focus-in/focus-out reporting |
| `\033[?2004l` | Disable bracketed-paste mode |
| `\033[?1049h\033[?1049l` | Enter-then-exit alternate screen (see below) |
| `\033[?25h`   | Show cursor |

**Rationale**: A remote program that enables mouse reporting, alternate screen,
or bracketed paste does not always clean up after itself — particularly when
the session ends due to a transport error or a signal. Without the reset, the
local terminal is left in a garbled state (invisible cursor, mouse clicks
producing escape characters in the next command, etc.). The sequences are
idempotent, so sending them after a clean exit is harmless.

**Alternate screen handling**: The `\033[?1049h\033[?1049l` pair (enter then
immediately exit the alternate screen) is used instead of a bare `\033[?1049l`
to avoid a viewport-jump bug observed on VTE/Ptyxis when no alternate screen
was active:

- **Remote program used the alternate screen** (e.g. vim, htop): `\033[?1049h`
  is already a no-op; `\033[?1049l` exits the alternate screen and restores the
  normal-screen cursor. Correct behaviour.
- **Remote program did not use the alternate screen** (e.g. a plain bash
  session): `\033[?1049h` saves the current cursor position and enters the
  alternate screen; `\033[?1049l` immediately exits and restores the same
  cursor position. Net effect: no-op, no viewport jump.

A bare `\033[?1049l` when no alternate screen was active caused the terminal
emulator to restore whatever cursor position it had saved from the last
`\033[?1049h` in the session — often the top of the visible area from a
previous full-screen program — resulting in the prompt overlaying old output.

**Caveat**: When run from inside a **tmux pane** or a **terminal multiplexer**,
the sequences are interpreted by the pane's own emulator before reaching the
outer terminal, so the outer program's screen is unaffected. When run from
inside a **terminal-emulator window** that itself displays a full-screen TUI
(e.g. neovim spawned directly in a terminal, not in a tmux pane), the
`?1049h?1049l` pair will switch that TUI back to its normal buffer when the
attach ends. To avoid this, use `--detach` to skip the attach entirely.

**Implementation**: `terminalResetSequence()` and `restoreWith()` in
`internal/cli/sandbox_connect.go`; called from `cliTerminal.MakeRaw`'s
restore closure only when the local endpoint is a TTY.

## stderr warning when `--gateway-insecure` is active

**Difference**: When `--gateway-insecure` or `OPENSHELL_GATEWAY_INSECURE` is
set, openshellctl prints a fixed warning to stderr before any network I/O. The
upstream Rust CLI applies the flag silently.

**Rationale**: The flag overrides `http.DefaultTransport` process-wide, which
means TLS certificate verification is disabled for **gateway connections and
OIDC token-endpoint exchanges** — not only the gRPC dial as the flag name
implies — and bearer tokens or client secrets may reach an impersonated server.
openshellctl's own `/auth/oidc-config` probe uses a clean transport captured
before `applyGatewayInsecureTransport()` runs and always verifies TLS. A
silent opt-in leaves users unaware that credential traffic may reach an
impersonated server. The warning makes the scope explicit. The warning goes to
stderr so that `-o json` stdout output remains parseable.

`OPENSHELL_*` environment variables are trusted input; a poisoned `.envrc` or
CI environment variable can enable this flag without any explicit flag
invocation, which is another reason to surface it.

**Implementation**: `insecureWarnText` const and `insecureWarning(bool) string`
in `internal/cli/root.go`; called from `PersistentPreRunE` in `NewRootCommand`
before `applyGatewayInsecureTransport()`. `TestNoPersistentPreRunEOverrideWithoutChaining`
guards against future subcommands silently dropping the hook.
