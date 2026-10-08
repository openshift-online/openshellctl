# `doctor` — connectivity/auth preflight

`openshellctl doctor` runs the same diagnostics the rosa-agent CronJobs' bash
steps (DNS for the issuer/gateway hosts, mint a token, whoami) used to do by
hand, natively in Go: one line per check, pass/fail/skip, with the exact next
command for any failure. It exists because an onboarding thread took four
people forty minutes to discover that one user had no secret in his
environment and another had no gateway metadata — `doctor` turns that into a
single command's output.

See the [README's "First run"](../README.md#first-run-openshellctl-doctor)
section for an all-green and a failing sample.

## The checks

Every invocation runs all 9 checks, in this order. A skipped check still
prints a line — "skip" is not an early exit, it's a first-class outcome that
explains why that check didn't run.

| # | Check | What it confirms | Skip rule | Exit code on failure |
|---|---|---|---|---|
| 1 | **Endpoint URL** | The resolved gateway endpoint normalizes cleanly (`gatewayconfig.NormalizeEndpoint`) | never | 2 (usage) |
| 2 | **DNS** | The endpoint's host resolves | Endpoint URL failed | 1 (generic) |
| 3 | **HTTP reachability** | `<endpoint>/auth/oidc-config` is reachable and returns well-formed JSON | DNS failed | 1 (generic) |
| 4 | **Credentials** | A token can actually be minted (static token, client-credentials exchange, or on-disk bundle) | never — this is the check everything else depends on | 3 (auth) |
| 5 | **Audience** | The minted token's audience includes the expected one (`--oidc-audience` → `metadata.oidc_audience` → `openshell-cli`) | Credentials failed | 3 (auth) |
| 6 | **Roles** | The minted token carries `openshell-user` or `openshell-admin` | Credentials failed | **7 (forbidden)** |
| 7 | **Expiry** | The minted token has a known, future expiry | Credentials failed | 3 (auth) |
| 8 | **OIDC config match** | The registered gateway's configured issuer/audience still matches what `/auth/oidc-config` actually serves (metadata drift) | nothing discovered (check 3 didn't run/failed) or nothing registered (endpoint-only invocation) | 1 (generic) |
| 9 | **Providers** | Every `--provider`/`-f` name or type exists on the gateway | nothing requested (no `--provider`/`-f`), or Credentials failed (no token to dial with) | 2 (usage) |

Checks 2-3 (DNS, HTTP reachability) run independently of check 4
(Credentials) — a DNS/HTTP problem against the discovery endpoint doesn't
necessarily mean token minting also fails (it may use a disk bundle or
static token reached a different way), so `doctor` shows both independently
rather than one failure hiding the other.

Roles failing gets its own exit code (7, not 3) because it predicts a real
`permission denied` the gateway would otherwise return on the very next
command — the same distinction Feature B's `ExitForbidden` makes for an
actual gateway response, applied preemptively here. Re-authenticating
(`token refresh`) cannot fix it; the hint says so and names the actual fix
(grant the role, or use a service account client that already has it).

## Flags

| Flag | Description |
|------|-------------|
| `-o`, `--output` | `table` (default), `json`, `yaml` — the structured formats are a real array, one object per check (`name`, `status`, `detail`, `nextStep`) |
| `--provider` | Provider name/type to check exists on the gateway (repeatable) |
| `-f`, `--file` | Manifest file to read `providerRefs` from, when `--provider` is not given (`-` for stdin) — same decode/validate path `sandbox create -f` uses |

`--provider` wins outright over `-f` when both are given, matching `sandbox
create`'s own flag-overrides-manifest precedence.

## Exit codes

`doctor` reuses the CLI's existing exit-code vocabulary (see the README's
[Exit codes](../README.md#exit-codes) table) — it never invents a new one.
When more than one check fails, the exit code is picked by priority: Roles
(7) first, then Credentials/Audience/Expiry (3), then Endpoint URL/Providers
(2), else 1 — so the exit code always reflects the most fundamental problem,
not just the first or last check that happened to fail.

## What `doctor` does not check

- It does not create, list, or delete sandboxes — it only confirms the
  preconditions for those commands to work.
- The Providers check confirms a provider *exists*; it does not validate the
  provider's own credentials are correct (that surfaces on first use).
- `--gateway-insecure` applies to `doctor`'s HTTP reachability check the same
  way it applies everywhere else in the CLI (one global toggle — see
  [`docs/gateway.md`](gateway.md)) — `doctor` does not add a second one.
