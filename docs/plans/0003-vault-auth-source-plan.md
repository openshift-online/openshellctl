# 0003 — native Vault source for `openshellctl` auth config — requirements

Status: draft, for alignment before implementation. Tracked upstream as [openshift-online/openshellctl#40](https://github.com/openshift-online/openshellctl/issues/40) — keep this file and that issue in sync; this file carries the full rationale and code-location detail, the issue carries the condensed version for GitHub.
Module: `github.com/openshift-online/openshellctl` (`pkg/auth`, `internal/cli`)
Author's intent: hand this to an implementing agent as-is; it does not need to re-derive the current behaviour, just build to the contracts below.

## 0. Scope

**In scope:** teach `openshellctl` to optionally source its own auth configuration (gateway, OIDC issuer/client-id/client-secret/audience/scopes) from a single Vault KV secret, as a third config source alongside the existing flags and `OPENSHELL_*` env vars. This is a generic CLI quality-of-life feature, not specific to any one team or workflow.

**Out of scope (explicitly, per discussion with the requester):** anything Jira- or ROSA-ticket-specific (ticket parsing, sandbox-name derivation, prompt templates, batch/interactive policy). That logic will be handled separately via skills/images and does not touch `openshellctl`. Do not add any Jira awareness to this repo as part of this plan. Also out of scope: Vault Kubernetes auth (for in-cluster/cron jobs with no `~/.vault-token` file) — worth a follow-up issue if that use case shows up, but not here.

### 0.1 Why

Today, two scripts in a user's `~/.bashrc.d/` do the following to let `openshellctl` authenticate as a service identity whose OIDC client credentials live in Vault:

```bash
# 97_rosa_agent_auth.bashrc — only defined inside a toolbox
rosa-agent-token() {
  vault login -method=oidc -non-interactive -no-print
  sleep 2
  secret=$(vault kv get -mount=osd-sre -field=hypershell-oidc-client-secret rosa-agent)
  client_id=$(vault kv get -mount=osd-sre -field=hypershell-oidc-client-id rosa-agent)
  export OPENSHELL_OIDC_CLIENT_ID="${client_id}"
  export OPENSHELL_OIDC_CLIENT_SECRET="${secret}"
}
```

`openshellctl` already natively supports `OPENSHELL_OIDC_CLIENT_ID` / `OPENSHELL_OIDC_CLIENT_SECRET` (`internal/cli/root.go`, `pkg/auth/resolve.go`) — the only thing the bash is doing is the Vault lookup and `export`. That's a generic, repeatable need (any team storing an OpenShell service identity's credentials in Vault hits the same problem), so it belongs in the tool, not in a per-user dotfile.

### 0.2 Decisions settled with the requester (2026-10-09)

- Vault integration is **native** (a real Vault API client inside `openshellctl`), not a shell-out to the `vault` binary and not a generic exec-credential-plugin hook.
- `openshellctl` does **not** perform `vault login` or any OIDC/browser flow against Vault itself. It expects the user already has a valid Vault session (`vault login` already run) and errors clearly if it can't find one — no login flow to build or maintain.
- Vault server address comes from the standard `VAULT_ADDR` env var (and other standard `VAULT_*` env vars Vault's own Go SDK already understands) — no new `openshellctl`-specific flag for the server address.
- The user supplies **which** Vault secret engine (KV mount) and **which** secret (path) hold the values, plus an optional field-name prefix, since the field-naming convention varies by team (today's is `hypershell-oidc-client-id`, `hypershell-oidc-client-secret`; another team might use a different prefix or none).
- This must cover the auth-relevant fields `openshellctl` already takes via flag/env — not just the client secret — so a team can put gateway, issuer, client id, client secret, and audience/scopes in one Vault secret if they want to.

### 0.3 Revisions after code-grounded review (2026-10-09)

A review of the first draft against the actual repo at commit `b53fe61` found four problems that change the contract below (not just wording):

1. The original draft's field-naming section and its `oidc.client_secret` wiring note, read together, implied the Vault *field name* itself might be `<prefix>-oidc.client_secret`. It must not be — the Vault field name is always hyphenated (`oidc-client-secret`); only the *viper key it gets written to* is dotted. §3 and §4.4 below are now explicit about which is which.
2. `clientSecretProvider` (`authwiring.go:26-40`) checks `viper.GetString("oidc.client_secret")` **before** falling back to `--client-secret-file`. A Vault-sourced default for that key would therefore win over an explicitly-passed `--client-secret-file`, inverting the promised flag-beats-Vault precedence. §4 now specifies skipping the Vault default for this key when `--client-secret-file` is set.
3. `gateway`/`gateway-endpoint` and `oidc-client-id`/`oidc.client_secret` are semantically paired, not independent — filling one half from Vault while the other half comes from a user flag can silently cross-wire config (dial one gateway's endpoint while loading another's metadata; use one client's ID with another client's secret). §3.1 now requires treating each pair atomically.
4. `token` is dropped from the Vault-fillable set entirely: `auth.Resolve` (`pkg/auth/resolve.go:72`) checks `StaticToken` before the client-credentials path, so a Vault-sourced token would override a user's own explicit `OPENSHELL_OIDC_CLIENT_SECRET` regardless of precedence intent — and tokens are short-lived JWTs, not something that belongs cached in Vault. The field count drops from 8 to 7.

Also incorporated: the Vault lookup must not run in root `PersistentPreRunE` (would make unrelated commands like `version` depend on a live Vault session — see §4.1), concrete exit codes tied to this repo's real `exitcode.go` scheme, a documented test-injection seam, and a dependency-weight trade-off note. All reflected in the sections below; this plan doc is now the single up-to-date source (§0.1–§0.2 above are kept for history, but §1 onward reflects the revised design).

## 1. Current auth config surface (what Vault must be able to fill)

From `internal/cli/root.go` (`registerPersistentFlags`) and `internal/cli/authwiring.go` (`resolveTokenSource`), the following viper keys drive auth resolution today, each already settable by flag and by `OPENSHELL_*` env var (via `envKeyReplacer`, `-`/`.` → `_`):

| Viper key | Flag | Existing env var | Used for |
|---|---|---|---|
| `gateway` | `--gateway` / `-g` | `OPENSHELL_GATEWAY` | named gateway resolution |
| `gateway-endpoint` | `--gateway-endpoint` | `OPENSHELL_GATEWAY_ENDPOINT` | endpoint-only resolution |
| `oidc-issuer` | `--oidc-issuer` | `OPENSHELL_OIDC_ISSUER` | client-credentials issuer override |
| `oidc-client-id` | `--oidc-client-id` | `OPENSHELL_OIDC_CLIENT_ID` | client-credentials client id |
| `oidc.client_secret` *(note the dot, not a flag — see §4.4 gotcha)* | — | `OPENSHELL_OIDC_CLIENT_SECRET` | client-credentials secret (also `--client-secret-file`) |
| `oidc-audience` | `--oidc-audience` | `OPENSHELL_OIDC_AUDIENCE` | client-credentials audience override |
| `oidc-scopes` | `--oidc-scopes` | `OPENSHELL_OIDC_SCOPES` | client-credentials scopes override (space-separated) |

**`token` (`--token` / `OPENSHELL_TOKEN`, the static bearer token) is deliberately excluded** — see §0.3 point 4. These seven are the complete set the new Vault source must be able to populate. Do not invent new semantics for any of them — Vault is purely an additional place to read the *same* values from.

## 2. New CLI/env surface

Add three new persistent flags (same pattern as the existing ones in `registerPersistentFlags`, bound via `bindViper` so they automatically get `OPENSHELL_*` env equivalents):

| Flag | Env var | Required together? | Meaning |
|---|---|---|---|
| `--vault-kv-mount` | `OPENSHELL_VAULT_KV_MOUNT` | yes, with `--vault-kv-path` | KV mount path, e.g. `osd-sre` |
| `--vault-kv-path` | `OPENSHELL_VAULT_KV_PATH` | yes, with `--vault-kv-mount` | secret path within the mount, e.g. `rosa-agent` |
| `--vault-field-prefix` | `OPENSHELL_VAULT_FIELD_PREFIX` | no (default `""`) | prepended to each field name when reading the secret, e.g. `hypershell` |

Flag names are `--vault-kv-mount`/`--vault-kv-path`, not `--vault-secret-engine`/`--vault-secret-path` (an earlier draft used the latter) — these match the vocabulary of `vault kv get -mount=<mount> <path>`, which is what a user will already know from the Vault CLI.

No `--vault-addr` flag. `openshellctl` must rely on Vault's own standard env vars for connection config (`VAULT_ADDR` at minimum; let `api.DefaultConfig()` — see §5 — pick up whatever else it already reads, e.g. `VAULT_CACERT`/`VAULT_SKIP_VERIFY`/`VAULT_NAMESPACE`, for free). **However**, `api.DefaultConfig()` silently falls back to `https://127.0.0.1:8200` when `VAULT_ADDR` is unset — fail fast with an explicit, clear error when the Vault source is activated but `VAULT_ADDR` is empty, rather than letting the user hit a confusing connection-refused error against localhost. Also check `cfg.Error` after calling `api.DefaultConfig()`: malformed `VAULT_*` env values (e.g. an unparsable `VAULT_CLIENT_TIMEOUT`) surface there, not as a returned error.

**Activation rule:** the Vault source is active only when both `--vault-kv-mount` and `--vault-kv-path` (flag or env) are non-empty. Setting exactly one and not the other is a usage error (`&UsageError{...}`, exit code 2, same family as the existing `root.SetFlagErrorFunc` path) — fail fast, don't silently ignore it. Setting neither must be a complete no-op: zero behavior change, zero Vault calls, for every existing user of `openshellctl`.

## 3. Field naming in the Vault secret

For each of the 7 logical fields in §1, the corresponding key inside the Vault secret's data is:

```
<prefix>-<field-suffix>        (prefix non-empty)
<field-suffix>                 (prefix empty, the default)
```

where `field-suffix` is always the **hyphenated** form: `gateway`, `gateway-endpoint`, `oidc-issuer`, `oidc-client-id`, `oidc-client-secret`, `oidc-audience`, `oidc-scopes`. This is true even for the field that ends up at viper key `oidc.client_secret` — the Vault field name is `oidc-client-secret` (or `<prefix>-oidc-client-secret`); the dot only appears on the viper-key side of the mapping (§4.4). Don't conflate the two.

(Today's real-world secret has `hypershell-oidc-client-id` and `hypershell-oidc-client-secret` under mount `osd-sre`, path `rosa-agent` — i.e. prefix `hypershell`.)

Normalize a trailing `-` on `--vault-field-prefix` before building keys, so `--vault-field-prefix hypershell-` and `--vault-field-prefix hypershell` produce the same lookups (`hypershell-oidc-issuer`, not `hypershell--oidc-issuer`).

A field absent from the secret is simply skipped, not an error — most Vault secrets will only carry a subset (e.g. just the two OIDC client-credentials fields), and a missing field falls through to whatever flag/env/default already applies, same as today. A field **present but not a string** (e.g. a nested object or number) is an error. Keys in the secret that don't match any known `<field-suffix>` (after stripping the configured prefix) are ignored.

### 3.1 Mutual exclusivity / field pairing

Two pairs must be treated atomically, not field-by-field, or Vault can silently produce a cross-wired config:

- **`gateway` + `gateway-endpoint`**: `gatewayconfig.Resolve` (`pkg/gatewayconfig/config.go:216-230`) takes the "endpoint given" branch whenever `Endpoint != ""`, and loads TLS/metadata for whatever `Name` happens to be set. If a user passes `-g other-gw` and Vault fills `gateway-endpoint`, requests get dialed at Vault's endpoint while loading `other-gw`'s metadata. **Rule:** if the user sets either half via flag/env, ignore both halves from Vault (don't call `viper.SetDefault` for either key).
- **`oidc-client-id` + `oidc.client_secret`**: `auth.Resolve` uses them as a pair for client-credentials. A user-set `OPENSHELL_OIDC_CLIENT_ID` combined with a Vault-sourced secret for a *different* client produces a confusing 401 at the gateway. **Rule:** same as above — either half set by the user (flag or env) disables Vault for both halves of this pair.

## 4. Precedence and wiring

Precedence per field, highest first: **explicit flag → `OPENSHELL_*` env var → Vault secret field → (existing default, e.g. `""`)**, subject to the pairing rule in §3.1. Vault is strictly a fallback source, never an override.

Implementation shape, to fit the existing architecture without restructuring `pkg/auth/resolve.go`:

1. New function, e.g. `applyVaultAuthSource(ctx context.Context) error` in `internal/cli` (or a thin wrapper around a `pkg/auth` helper — implementer's call). **Do not** call this unconditionally from root `PersistentPreRunE` — see §4.1 for why and where it actually belongs.
2. If the activation rule (§2) isn't met, return immediately — no Vault client is even constructed.
3. Otherwise, read the secret **once** per invocation (a single KV read, not one per field — see §5) and, for each of the 7 fields present in the secret and not excluded by the §3.1 pairing rule, call `viper.SetDefault(viperKey, value)` — **not** `viper.Set`. `viper.SetDefault` is exactly the right primitive here: viper's own precedence already puts explicit `Set`/flag/env above defaults, so this gets the precedence rule for free with no extra conditionals, and every downstream reader (`resolveTokenSource`, `gateway.go`, etc.) needs zero changes — they just see a value that came from somewhere.
4. **Gotcha to preserve exactly:** the Vault field `oidc-client-secret` (hyphenated, per §3) must be written to viper key `oidc.client_secret` (dotted) — matching the existing asymmetry in `clientSecretProvider` (`authwiring.go`), which reads `viper.GetString("oidc.client_secret")` directly rather than through a bound flag. Every other field uses its hyphenated key as-is for both the Vault field name and the viper key.
5. **Second gotcha:** `clientSecretProvider` (`authwiring.go:26-40`) checks `viper.GetString("oidc.client_secret")` *before* falling back to reading `--client-secret-file`. Because that check has no way to distinguish "set via env" from "set via `SetDefault`", a Vault-sourced default for `oidc.client_secret` would win over an explicitly-passed `--client-secret-file`, even though a flag should beat Vault. **Do not** call `viper.SetDefault("oidc.client_secret", ...)` when `viper.GetString("client-secret-file")` is non-empty — treat `--client-secret-file` as occupying that pairing slot the same way an explicit `OPENSHELL_OIDC_CLIENT_ID`/`OPENSHELL_OIDC_CLIENT_SECRET` would.
6. Never call `viper.SetDefault` with an empty string (don't overwrite "unset" with "unset" — harmless either way, but skip it to keep debug logging in §7 meaningful).

### 4.1 Where the lookup runs

Not in root `PersistentPreRunE` — that would make commands with no auth dependency at all (`version`, `policy lint`, `gateway list`, etc.) require a live Vault session whenever `OPENSHELL_VAULT_KV_MOUNT`/`OPENSHELL_VAULT_KV_PATH` happen to be set in the environment (e.g. from a user's dotfiles). Instead, load lazily and memoize (once per process) at the existing call sites that actually resolve auth or gateway config:

- `resolveTokenSource` (`internal/cli/authwiring.go`)
- `runLogin` (`internal/cli/login.go:34`)
- the gateway commands that call `gatewayconfig.Resolve` directly (`internal/cli/gateway.go:142`, `:207`)
- `internal/cli/sandbox_sshconfig.go:31`

**Open design question to settle during implementation, not silently:** `gateway add` (`internal/cli/gateway.go:227-228`) persists `oidc-client-id`/`oidc-audience` into `metadata.json` on disk via `gatewayconfig.NewMetadata`. Decide explicitly whether Vault-sourced values should be allowed to get written into persisted gateway metadata by that call, or whether that one call site should read the pre-Vault viper values instead. Either choice is acceptable; silently inheriting whatever `gateway add` already does is not — document the choice in the PR.

## 5. Vault client requirements

- New dependency: `github.com/hashicorp/vault/api`. Latest stable as of this writing is **v1.23.0** (confirm still current at implementation time; record the version used in the PR description per this repo's convention).
- Build the client with `api.NewClient(api.DefaultConfig())`, checking `cfg.Error` (see §2) and failing fast if `VAULT_ADDR` is unset.
- **Token resolution (read-only, no login flow):**
  1. `VAULT_TOKEN` env var, if non-empty.
  2. Otherwise, `~/.vault-token` (the file the `vault login` CLI's default internal token helper writes), trimmed of whitespace. The official SDK only reads `VAULT_TOKEN` itself — reading the token-helper file is on us to implement.
  3. If neither yields a non-empty token: hard error, do not proceed, exit code `ExitAuth` (3, matching `internal/cli/exitcode.go`'s existing constant). Message must name both places checked and tell the user to run `vault login`, and mention that custom Vault token helpers (anything other than the default `~/.vault-token` file) aren't supported, e.g.: `"no Vault session found (checked $VAULT_TOKEN and ~/.vault-token) — run \`vault login\` first (custom token helpers are not supported)"`.
  4. Take the home-directory lookup and environment access as injected inputs (not real `$HOME`/`os.Getenv`) so this is testable hermetically — don't read the real filesystem/env from inside the function under test.
  - **Verify at implementation time** whether `api.NewClient(api.DefaultConfig())` already populates the client's token from `VAULT_TOKEN` on its own (behavior has varied across SDK versions) — if so, step 1 may already be free; step 2 (`~/.vault-token`) is definitely not something the `api` package does automatically and must be implemented explicitly.
  - Do not pre-validate the token with a separate `LookupSelf` call — let the KV read itself fail, and map the resulting 403/404 to the errors in §6. One network round trip, not two.
- **KV version:** assume KV v2 (the modern default, and what the real-world `osd-sre` mount already is) and use the SDK's `client.KVv2(mount).Get(ctx, path)` helper. Auto-detecting v1-vs-v2 mounts is a nice-to-have, not required for this plan. If the secret read 404s, the error message should prompt the user to check whether the mount is actually KV v2 (a v1 mount 404s on the `data/` sub-path v2 uses).
- Fetch the secret **once per process invocation** (not once per field, not once per call site in §4.1) — memoize for the lifetime of the process.
- Use `cmd.Context()` with a short, explicit timeout for the KV read — the SDK's own default is 60s plus retries, far too slow for a CLI command that should fail fast on a Vault problem.
- Abstract the read behind a small interface (mirroring the existing `Exchanger` pattern in `pkg/auth/clientcreds.go`), e.g.:
  ```go
  type VaultReader interface {
      ReadSecret(ctx context.Context, mount, path string) (map[string]string, error)
  }
  ```
  so unit tests use a fake and never touch a network or a real Vault server — this repo's tests never touch the network, full stop (see `docs/plans/0002-openshell-go-client-plan.md` §0 hard rules), and this feature doesn't get an exception.

### 5.1 Dependency trade-off

A KV v2 read is, at the HTTP level, a single `GET $VAULT_ADDR/v1/<mount>/data/<path>` with a bearer token header — `github.com/hashicorp/vault/api` is a much heavier way to get there (pulls in `retryablehttp`, `hcl`, `go-jose`, and more transitively). The recommendation is to use the SDK anyway, for correct handling of `VAULT_CACERT`/`VAULT_NAMESPACE`/the rest of the standard `VAULT_*` env vars rather than hand-rolling TLS/namespace handling — but this is a real trade-off for a tool whose own design doc (`0002`) emphasizes a lightweight client, so record it explicitly in the PR description rather than adding the dependency silently.

## 6. Error handling

Map errors to this repo's existing exit-code scheme (`internal/cli/exitcode.go`) rather than letting them fall through to the generic `ExitError` (1):

| Condition | Exit code | Hint |
|---|---|---|
| Only one of `--vault-kv-mount`/`--vault-kv-path` set | `ExitUsage` (2) | — |
| Neither set | — | no-op; existing behavior unchanged |
| Both set, no Vault token resolvable | `ExitAuth` (3) | names both places checked, suggests `vault login` |
| Both set, Vault returns 403/permission denied | `ExitForbidden` (7) | needs a KV v2 policy granting `<mount>/data/<path>` — not fixed by re-authenticating |
| Both set, mount or path not found (404) | `ExitNotFound` (4) or `ExitAuth`, implementer's call — pick whichever existing typed error this naturally maps through | secret missing at `<mount>/<path>`; also suggests checking whether the mount is actually KV v2 |
| Secret fetched, some expected field missing | — | not an error; falls through to existing `auth.Resolve` behavior/errors downstream, same as if the field had never been set anywhere |
| Secret field present but non-string | error | — |
| Secret value used | — | never logged, at any verbosity — matching the existing "env value is never logged" discipline for `oidc.client_secret` |

None of the three error rows above should get the existing generic `refreshHint` (`exitcode.go`'s `refreshHint` constant — re-running `token refresh` fixes none of them). The existing `ErrNoCredentials` hint (`hintFor`, `exitcode.go`) should also mention the new `--vault-kv-mount`/`--vault-kv-path` flags as an option alongside the existing advice.

Debug/`-v` logging may name *which fields* were sourced from Vault (e.g. `"oidc-client-id: from vault (osd-sre/rosa-agent)"`) but must never print the value for `oidc-client-secret`.

## 7. Testing requirements

All via the `VaultReader` fake — no real Vault server, no `vault` binary, no network, consistent with the repo's existing test discipline.

- `cliDeps` (`internal/cli/deps.go`) is explicitly documented as being read in exactly one place (`resolveAuth`). Either add a separate, clearly-documented seam for injecting the fake `VaultReader` (preferred — don't overload a struct whose doc comment makes a specific, load-bearing claim about being read in one place), or update that comment deliberately if `cliDeps` is reused. Don't leave the comment wrong either way.
- Any test that calls `viper.SetDefault` (directly or via the code under test) must `viper.Reset()` and `t.Cleanup(viper.Reset)` — global viper state leaks between tests otherwise. Follow the existing pattern already used in `tokenflow_test.go:102`.

Required table coverage:
- Activation: neither flag set → no-op (verify no `VaultReader` call happens at all, not even construction); only one set → `UsageError`; both set → reader invoked exactly once per process/command.
- Field-prefix mapping: prefix empty → bare field names; prefix set → `<prefix>-<field>` names; a trailing `-` on the configured prefix doesn't produce a double hyphen.
- Precedence: flag beats Vault; env beats Vault; Vault fills a field left empty by both; a secret containing all 7 fields doesn't clobber an explicitly-set `--gateway`.
- The `oidc-client-secret` → `oidc.client_secret` key-name case (§4.4) — a dedicated test, since it's the one asymmetric case and the easiest to get wrong.
- The `--client-secret-file` vs. Vault-sourced-secret precedence case (§4.5) — a dedicated test, since this is the specific bug the first draft of this plan missed.
- Both mutual-exclusivity pairing rules (§3.1): gateway-name-set-by-user + Vault-sourced endpoint; client-id-set-by-user + Vault-sourced secret — confirm Vault is skipped for *both* members of the pair in each case.
- Partial secrets: only the two OIDC client-credentials fields present (today's real shape) — everything else stays whatever flags/env already provide.
- Secret contents: a non-string field value is an error; an unrecognized key in the secret is silently ignored.
- Error paths: no token found (both `VAULT_TOKEN` unset and `~/.vault-token` absent/empty); reader returns a 403-shaped error; reader returns a 404-shaped error. Assert on user-facing message content, not just error type/exit code, since these are the messages a human reads when something's misconfigured.
- `token` is confirmed absent from anything the Vault source can populate (e.g. a secret containing a `token` field has no effect).

## 8. Docs

Update `README.md`, `docs/gateway.md`, and `docs/troubleshooting.md` (all three already exist in this repo) — not just `--help` output.

## 9. What this removes from the bash, once shipped

Not this repo's work to change, but worth stating as the acceptance signal for "QoL for CLI users":

- `97_rosa_agent_auth.bashrc` becomes entirely unnecessary and can be deleted outright — no more `vault login -method=oidc`, no more manual `vault kv get` + `export`.
- The `command -v rosa-agent-token` guard and the `rosa-agent-token || return 1` call in `98_openshell_jira_agent.bashrc`'s `_jira_agent_run` (lines ~141–144, 160) go away. In their place, that script just needs to pass `openshellctl` the three new top-level flags (or rely on them already being exported as env vars in the user's shell) *before* the `sandbox` subcommand, e.g.:
  ```
  openshellctl --vault-kv-mount osd-sre --vault-kv-path rosa-agent --vault-field-prefix hypershell \
    sandbox create --name "${sandbox_name}" --from "${_JIRA_AGENT_IMAGE}" ...
  ```
  Note these are flags to `openshellctl` itself (control-plane auth), not `--env` values injected into the created sandbox — don't confuse the two in the eventual bash cleanup.

## 10. Acceptance criteria

- [ ] New flags/env vars exist, documented in `--help`, `README.md`, `docs/gateway.md`, `docs/troubleshooting.md`.
- [ ] Vault source is fully opt-in: an `openshellctl` invocation with none of the three new flags/env vars behaves byte-for-byte as before — no new network calls, no new error paths reachable.
- [ ] Only commands that resolve auth touch Vault — `version`, `policy lint`, etc. are unaffected even with Vault flags/env set.
- [ ] All 7 fields in §1 are fillable from Vault, including the `oidc.client_secret` key-name case (§4.4), the `--client-secret-file` precedence case (§4.5), and both mutual-exclusivity pairing rules (§3.1).
- [ ] `token` is not Vault-fillable.
- [ ] No test in the new code touches a real network or a real Vault server; `viper.Reset()`/`t.Cleanup` discipline followed throughout.
- [ ] `go.mod`/`go.sum` updated for the new Vault SDK dependency; version used and the §5.1 dependency-weight trade-off recorded in the PR description.
- [ ] Error messages for the "no token", "403", "404", and "only one flag set" cases are asserted in tests by content, not just by type, and map to `ExitAuth`/`ExitForbidden`/`ExitUsage` as specified in §6.
