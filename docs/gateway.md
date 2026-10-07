# `gateway` — registering an openshellctl gateway

`openshellctl gateway add` registers a gateway's `gateways/<name>/metadata.json`
natively, without the upstream Rust `openshell` binary. This document covers
the on-disk metadata schema, how `gateway add` infers (or rejects) an auth
mode, and where this intentionally differs from upstream.

## The `metadata.json` schema

`Metadata` (`pkg/gatewayconfig/metadata.go`) mirrors upstream's
`GatewayMetadata` (`metadata.rs:14-74`):

| Field | JSON key | Written by `gateway add`? |
|---|---|---|
| `Name` | `name` | always |
| `GatewayEndpoint` | `gateway_endpoint` | always (the normalized endpoint) |
| `IsRemote` | `is_remote` | always `true` |
| `GatewayPort` | `gateway_port` | always `0` |
| `AuthMode` | `auth_mode` | always `"oidc"` |
| `OIDCIssuer` | `oidc_issuer` | always (explicit `--oidc-issuer`, or discovered) |
| `OIDCClientID` | `oidc_client_id` | when `--oidc-client-id` is given |
| `OIDCAudience` | `oidc_audience` | when `--oidc-audience` is given, or discovered |
| `OIDCScopes` | `oidc_scopes` | when `--oidc-scopes` is given |
| `RemoteHost`, `ResolvedHost` | `remote_host`, `resolved_host` | never (upstream-only fields) |
| `EdgeTeamDomain`, `EdgeAuthURL` | `edge_team_domain`/`cf_team_domain`, `edge_auth_url`/`cf_auth_url` | never (Cloudflare Access gateways — out of scope) |
| `VMDriverStateDir` | `vm_driver_state_dir` | never |

`IsRemote: true` / `GatewayPort: 0` matches the convention the rosa-agent
CronJobs' own hand-written heredoc already uses for an endpoint-addressed
remote gateway (see `pkg/gatewayconfig/metadata_marshal_test.go`'s golden
fixture, sourced from `openshift-online/rosa-agent`).

`Marshal()` reproduces the exact byte layout `serde_json::to_string_pretty`
would (2-space indent, struct-field order, no HTML escaping, no trailing
newline) for whatever fields are set — a `gateway add`-written `metadata.json`
is byte-for-byte identical in formatting to one an upstream-equivalent writer
would produce for the same field values. It always sets `oidc_client_id`
explicitly (defaulting to `openshell-cli` when `--oidc-client-id` isn't
given, printing a warning when a client secret is also present — see
"Differences from the upstream Rust CLI" below).

## Auth mode inference

Upstream infers an auth mode for a gateway whose `metadata.json` omits
`auth_mode` entirely: `https://` endpoints are treated as mTLS, `http://`
endpoints as plaintext (`commands/gateway.rs:581-587`). This inference exists
for **reading** metadata that predates the `auth_mode` field, or that was
hand-written without it (`pkg/gatewayconfig/mtls.go`'s `TLSMaterialFor` is
the read-side consumer, locating `mtls/ca.crt`/`tls.crt`/`tls.key`).

`gateway add` never relies on that inference when **writing**: it always sets
`auth_mode: "oidc"` explicitly, and only after confirming an OIDC issuer is
available (via `--oidc-issuer` or a successful `/auth/oidc-config` probe). If
neither is available, `gateway add` fails with a clear error instead of
silently falling back to an inferred mTLS/plaintext mode — see "Differences
from the upstream Rust CLI" below.

## Differences from the upstream Rust CLI

- **OIDC gateways only.** `gateway add` has no equivalent of an explicit
  `--mtls`/edge-gateway registration path. A gateway that doesn't answer
  `/auth/oidc-config` and has no `--oidc-issuer` override fails with a typed
  `EdgeGatewayUnsupportedError` (exit code 2) rather than registering an
  under-specified or incorrect auth mode.
- **No default browser login when a secret is absent and
  `OPENSHELL_NO_BROWSER` is set.** Upstream's `gateway add` always opens a
  browser when no client secret is configured, which fails for a
  service-account client (no redirect URIs; `client_credentials` only).
  `gateway add` instead: registers the gateway, prints a hint to log in
  separately (`gateway login`) or set `OPENSHELL_OIDC_CLIENT_SECRET`, and
  exits `0`. This interim behavior is tracked by
  [ROSAENG-68835](https://redhat.atlassian.net/browse/ROSAENG-68835) (a
  time-boxed spike deciding whether to add full device-code login parity);
  when a secret is absent but `OPENSHELL_NO_BROWSER` is unset,
  `gateway add` falls back to the normal interactive browser flow, matching
  upstream's behavior for a human user.
- **Real `expires_at` on every authenticated registration.** The rosa-agent
  CronJobs' hand-written Python token writer omits `expires_at` entirely;
  `gateway add`'s client-credentials path always writes it (via
  `auth.WriteBundle`), so `token show`/`token refresh` can report accurate
  expiry without a round trip to the gateway.
- **Atomic, permission-safe writes.** `metadata.json` and `oidc_token.json`
  are written via `OSWriter` (temp file + rename), always `0600` with `0700`
  parent directories — not shell `cat > file <<EOF` + a separate `chmod`
  step, which has a brief window where the file exists world-readable.
- **Automatic rollback on authentication failure.** If `gateway add` writes
  `metadata.json` but then fails to authenticate (wrong secret, wrong client
  ID, unreachable issuer), it removes the registration and restores whichever
  gateway was previously active (or clears `active_gateway` if there wasn't
  one) before returning the error — a corrected retry doesn't need a manual
  `gateway remove` first. Pass `--force` to intentionally overwrite an
  existing registration under the same name (e.g. to fix a stale endpoint).
- **`gateway remove` never touches `mtls/` or `last_sandbox`.** It only
  removes `metadata.json` and `oidc_token.json` — the two files this CLI
  itself can recreate. mTLS certificates are typically admin-issued and not
  recoverable from the CLI, so removing a registration to fix a typo and
  re-adding it must never destroy them.
- **`OPENSHELL_NO_BROWSER` is a real boolean**, bound the same way every
  other `OPENSHELL_*` flag/env pair in this CLI is (via `--no-browser` and
  viper): `0`/`false` (case-insensitive) means "browser allowed," not merely
  "the variable is unset."
