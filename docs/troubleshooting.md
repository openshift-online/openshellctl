# Troubleshooting auth onboarding

This document covers three honesty fixes to `token refresh` and `token show`
(ROSAENG-68828): cases where the CLI used to report success, or give an
unhelpful hint, when it actually hadn't done anything useful. All three are
fully reproducible offline — no gateway connection required.

## `token refresh` printed a fake success with no credentials configured

**Symptom**: running `openshellctl token refresh` against a gateway with no
static token, no client secret, no OIDC token bundle on disk, and no mTLS
material printed something like:

```
refreshed token for subject "" (expires unknown)
```

— a "success" message with an empty subject and no expiry, because nothing
was actually refreshed.

**Why**: `pkg/auth.Resolve`'s fallback case for an unset or mTLS auth mode
used to return a working-looking `NoAuthSource` unconditionally, even when
the gateway had no TLS material and nothing else was configured. `token
refresh` then happily printed its success message for that empty token.

**The fix**: `Resolve` now only returns that no-bearer-token source when TLS
material is actually present for a resolved mTLS gateway (`ResolveInput.
TLSPresent`). Otherwise it returns `auth.ErrNoCredentials`, which `token
refresh` wraps as `auth.ErrNothingToRefresh`.

**What to expect now**:

```
$ openshellctl token refresh --gateway-endpoint https://nowhere.invalid
Error: nothing to refresh: no credentials configured
```

exits with code 3 (see the exit-code table in the main README).

A resolved gateway with an explicit `auth_mode: none` or `plaintext` (a
legitimate, intentional no-auth configuration) also returns "nothing to
refresh" rather than a fake success — there is no openshellctl-managed
bearer token for either mode.

> **Note on the fully-unconfigured case**: running `openshellctl token
> refresh` with *zero* flags and no gateway registered at all does not hit
> this path — `gatewayconfig.Resolve` already, correctly, fails earlier with
> its own "no active gateway" error (exit 4) before auth resolution is ever
> attempted. That earlier check is unrelated to this fix and was left as-is.

## `token refresh --write` silently did nothing when there was nowhere to write to

**Symptom**: `openshellctl token refresh --write` against a gateway resolved
only by `--gateway-endpoint` (not a named, registered gateway) printed a
warning and exited 0 — the refreshed token was never actually persisted
anywhere.

**Why**: there is no `gateways/<name>/` directory to write `oidc_token.json`
into unless the gateway was registered with `gateway add`. The old code
detected this but only warned.

**The fix**: `--write` now fails fast with a usage error naming the fix:

```
$ openshellctl token refresh --write --gateway-endpoint https://nowhere.invalid
Error: --write requires a named, registered gateway; register one first: openshellctl gateway add <endpoint> --name <name>
```

exits with code 2. Register the gateway first (see [`gateway.md`](gateway.md)), then retry with `--write`.

## A permission-denied response got the same hint as an expired token

**Symptom**: a gateway call failing with a permission-denied response (the
caller is authenticated, but lacks a required role) printed the same
"Hint: try `openshellctl token refresh`" as an actually-expired token —
unhelpful, since obtaining a new token changes nothing about what roles are
granted to the subject.

**The fix**: permission-denied responses (`gateway.PermissionDeniedError`)
now get their own exit code, 7, and their own hint pointing at the real
problem:

```
$ openshellctl sandbox list
Error: permission denied: role "openshell-admin" required
Hint: you are authenticated, but the gateway denied this action due to insufficient permissions. Ask an administrator to grant the required role — obtaining a new token will not help.
```

`token refresh` is no longer suggested for this case. Contact whoever
administers role assignments on the gateway.
