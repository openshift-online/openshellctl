# openshellctl

Lightweight Go client and CLI for [NVIDIA OpenShell](https://github.com/NVIDIA/OpenShell) gateways. A pure-Go reimplementation of the upstream Rust CLI, built for embedding in operators and CI pipelines without requiring any external binaries.

## Installation

```bash
go install github.com/openshift-online/openshellctl/cmd/openshellctl@latest
```

Or build from source:

```bash
git clone https://github.com/openshift-online/openshellctl.git
cd openshellctl
make build          # binary at ./openshellctl
```

## Quick start

```bash
# Point at a gateway (env vars or config file)
export OPENSHELL_GATEWAY_ENDPOINT=https://gateway.example.com
export OPENSHELL_TOKEN=$(cat /path/to/token)

# Create a sandbox and connect
openshellctl sandbox create --from python
openshellctl sandbox connect my-sandbox

# Run a one-shot command
openshellctl sandbox exec --name my-sandbox -- python3 -c "print('hello')"

# Upload code, run it
openshellctl sandbox upload ./myproject:/workspace my-sandbox
openshellctl sandbox exec --name my-sandbox -- python3 /workspace/main.py

# Clean up
openshellctl sandbox delete my-sandbox
```

## Authentication

openshellctl supports three authentication methods:

1. **Bearer token** (simplest): pass `--token <JWT>` or set `OPENSHELL_TOKEN`.
2. **OIDC client credentials**: configure `--oidc-issuer`, `--oidc-client-id`, and `--client-secret-file` (or set their env equivalents). The CLI exchanges the credentials for a JWT and refreshes automatically.
3. **Disk-bundle** (Rust CLI compat): reads `oidc_token.json` from the gateway config directory (`$XDG_CONFIG_HOME/openshell/gateways/<name>/`). Supports refresh-token rotation.

Token management commands:

```bash
openshellctl token show               # display token subject, expiry, roles
openshellctl token refresh             # force a token refresh
openshellctl token refresh --write     # refresh and persist to oidc_token.json
openshellctl token inspect <JWT>       # decode a JWT's claims (no sig verify)
```

If a command fails with an expired or invalid token, openshellctl prints:

```
Error: invalid token: ExpiredSignature
Hint: try `openshellctl token refresh` to obtain a new token.
```

## Configuration

Configuration is resolved in priority order:

1. CLI flags (highest)
2. Environment variables (`OPENSHELL_*`)
3. Config file (`$XDG_CONFIG_HOME/openshellctl/config.yaml`, override with `--config`)
4. Gateway config dirs (`$XDG_CONFIG_HOME/openshell/gateways/<name>/`)

### Global flags

These flags apply to every command:

| Flag | Env var | Description |
|------|---------|-------------|
| `--gateway`, `-g` | `OPENSHELL_GATEWAY` | Gateway name (selects config directory) |
| `--gateway-endpoint` | `OPENSHELL_GATEWAY_ENDPOINT` | Gateway gRPC endpoint URL |
| `--gateway-insecure` | `OPENSHELL_GATEWAY_INSECURE` | Skip TLS verification |
| `--workspace` | `OPENSHELL_WORKSPACE` | Workspace name (default: `default`) |
| `--token` | `OPENSHELL_TOKEN` | Bearer token (overrides OIDC) |
| `--config` | — | Config file path |
| `--verbose`, `-v` | — | Increase verbosity (repeat for more) |
| `--no-color` | `NO_COLOR` | Disable coloured output |
| `--oidc-issuer` | — | OIDC issuer URL override |
| `--oidc-client-id` | — | OIDC client ID override |
| `--oidc-audience` | — | OIDC audience override |
| `--oidc-scopes` | — | OIDC scopes (space-separated) |
| `--client-secret-file` | — | File containing the OIDC client secret |
| `--token-leeway` | — | Token expiry leeway (default: `30s`) |
| `--write-token` | — | Write refreshed tokens to disk (Rust CLI schema) |

## Commands

### `sandbox create`

Create a sandbox and optionally connect to it.

```bash
# Create from a community image
openshellctl sandbox create --from python
openshellctl sandbox create --from python --name my-sandbox

# Create with GPU
openshellctl sandbox create --from pytorch --gpu       # driver default count
openshellctl sandbox create --from pytorch --gpu 2     # explicit count

# Create from a full image reference
openshellctl sandbox create --from ghcr.io/org/image:tag

# Create from a manifest file
openshellctl sandbox create -f sandbox.yaml
cat sandbox.yaml | openshellctl sandbox create -f -

# Create with uploads and port forwarding
openshellctl sandbox create --from python \
  --upload ./src:/workspace \
  --upload ./data:/data \
  --forward 8080 \
  --no-git-ignore

# Create and run a command
openshellctl sandbox create --from python -- python3 main.py

# Create without attaching
openshellctl sandbox create --from python --detach

# Output as JSON/YAML instead of table
openshellctl sandbox create --from python -o json
```

#### Create flags

| Flag | Description |
|------|-------------|
| `--from` | Image reference or community name (e.g. `python`, `pytorch`) |
| `--name` | Sandbox name (auto-generated if omitted; max 19 chars) |
| `-f`, `--file` | Manifest YAML file (`-` for stdin) |
| `--gpu [COUNT]` | Request GPU resources (bare = driver default, or specify count) |
| `--cpu` | CPU request (e.g. `500m`, `2`) |
| `--memory` | Memory request (e.g. `1Gi`, `512Mi`) |
| `--env` | Environment variable `KEY=VALUE` (repeatable) |
| `--label` | Label `key=value` (repeatable) |
| `--tty` | Force pseudo-terminal allocation |
| `--no-tty` | Disable pseudo-terminal (last-wins with `--tty`) |
| `--upload` | Upload `local[:dest]` after create (repeatable) |
| `--no-git-ignore` | Disable `.gitignore` filtering for uploads |
| `--forward` | Forward `[bind:]port` to sandbox |
| `--detach` | Return after create without attaching |
| `--provider` | Provider to attach (repeatable) |
| `--auto-providers` | Enable auto provider detection |
| `--no-auto-providers` | Disable auto provider detection |
| `--no-credential-warnings` | Suppress credential exposure warnings |
| `--approval-mode` | Proposal approval mode: `manual` (default) or `auto` |
| `--no-keep` | Delete the sandbox after the session ends |
| `--driver-config-json` | Driver config as inline JSON |
| `--policy` | Sandbox policy file (env: `OPENSHELL_SANDBOX_POLICY`) |
| `-o`, `--output` | Output format: `table` (default), `json`, `yaml` |

### `sandbox get`

Show details of a single sandbox.

```bash
openshellctl sandbox get my-sandbox
openshellctl sandbox get my-sandbox -o json
openshellctl sandbox get my-sandbox -o yaml
openshellctl sandbox get              # uses last-used sandbox
```

| Flag | Description |
|------|-------------|
| `-o`, `--output` | Output format: `table` (default), `json`, `yaml` |

### `sandbox list`

List sandboxes in the current workspace.

```bash
openshellctl sandbox list
openshellctl sandbox list -o json
openshellctl sandbox list --all-workspaces
openshellctl sandbox list --names          # one name per line
openshellctl sandbox list --ids            # one ID per line
openshellctl sandbox list --selector "gpu=true"
openshellctl sandbox list --limit 10 --offset 20
```

| Flag | Description |
|------|-------------|
| `-o`, `--output` | Output format: `table` (default), `json`, `yaml` |
| `--all-workspaces` | List across all workspaces |
| `--names` | Print names only (one per line) |
| `--ids` | Print IDs only (one per line) |
| `--selector` | Label selector filter |
| `--limit` | Max results (default: 100) |
| `--offset` | Result offset for pagination |

### `sandbox delete`

Delete one or more sandboxes.

```bash
openshellctl sandbox delete my-sandbox
openshellctl sandbox delete sb-1 sb-2 sb-3
openshellctl sandbox delete --all
openshellctl sandbox delete my-sandbox --wait
openshellctl sandbox delete my-sandbox --wait --wait-timeout 2m
```

| Flag | Description |
|------|-------------|
| `--all` | Delete all sandboxes in the workspace |
| `--wait` | Wait for deletion to complete |
| `--wait-timeout` | Wait timeout (default: `5m`) |

### `sandbox stop` / `sandbox start`

Stop or start a sandbox.

```bash
openshellctl sandbox stop my-sandbox
openshellctl sandbox start my-sandbox
openshellctl sandbox stop               # uses last-used sandbox
openshellctl sandbox start              # uses last-used sandbox
```

### `sandbox exec`

Run a command inside a sandbox. The command must follow `--`.

```bash
openshellctl sandbox exec --name my-sandbox -- ls -la
openshellctl sandbox exec --name my-sandbox -- python3 -c "print('hello')"

# With environment variables
openshellctl sandbox exec --name my-sandbox --env FOO=bar -- env

# With working directory
openshellctl sandbox exec --name my-sandbox --workdir /workspace -- make build

# With timeout (seconds)
openshellctl sandbox exec --name my-sandbox --timeout 60 -- long-running-task

# Force/disable TTY
openshellctl sandbox exec --name my-sandbox --tty -- bash
openshellctl sandbox exec --name my-sandbox --no-tty -- cat /etc/os-release

# Interactive mode (stdin is forwarded when --tty + terminal detected)
openshellctl sandbox exec --name my-sandbox --tty -- bash
```

| Flag | Description |
|------|-------------|
| `-n`, `--name` | Sandbox name (defaults to last-used) |
| `--env` | Environment variable `KEY=VALUE` (repeatable) |
| `--workdir` | Working directory inside the sandbox |
| `--timeout` | Timeout in seconds (0 = none) |
| `--tty` | Force pseudo-terminal allocation |
| `--no-tty` | Disable pseudo-terminal (last-wins with `--tty`) |

The process exit code matches the remote command's exit code.

### `sandbox connect`

Open an interactive shell session to a sandbox.

```bash
openshellctl sandbox connect my-sandbox
openshellctl sandbox connect             # uses last-used sandbox
```

Detach from the session with **Ctrl-P Ctrl-Q** (the connection closes cleanly and exits 0).

### `sandbox upload`

Upload files or directories to a sandbox.

```bash
openshellctl sandbox upload ./myproject my-sandbox
openshellctl sandbox upload ./myproject:/workspace my-sandbox
openshellctl sandbox upload ./file.txt:/remote/path/file.txt my-sandbox
openshellctl sandbox upload ./src:/workspace --no-git-ignore my-sandbox
```

The spec is `LOCAL[:DEST]`. If `DEST` is omitted, files are placed in the sandbox's home directory. Directory uploads use scp-like semantics (`./myproject` → `~/myproject/`).

By default, `.gitignore` rules are respected for directory uploads. Use `--no-git-ignore` to include all files.

| Flag | Description |
|------|-------------|
| `--no-git-ignore` | Disable `.gitignore` filtering for directory uploads |

### `sandbox download`

Download files or directories from a sandbox.

```bash
openshellctl sandbox download /workspace/output my-sandbox
openshellctl sandbox download /workspace/output:./local-dir my-sandbox
```

The spec is `REMOTE[:DEST]`. If `DEST` is omitted, files are placed in the current directory.

### `sandbox ssh-config`

Print an SSH configuration block for use with the upstream `openshell` binary.

```bash
openshellctl sandbox ssh-config my-sandbox
openshellctl sandbox ssh-config          # uses last-used sandbox
```

This outputs a block suitable for `~/.ssh/config`. It requires the upstream `openshell` binary for the ProxyCommand.

### `sandbox provider`

Manage providers attached to a sandbox.

```bash
# List providers
openshellctl sandbox provider list my-sandbox

# Attach a provider
openshellctl sandbox provider attach my-sandbox github

# Detach a provider
openshellctl sandbox provider detach my-sandbox github
```

Attach and detach use optimistic concurrency — if another operation modifies the sandbox concurrently, you will see:

```
Failed to attach provider: sandbox was modified by another operation.
Please retry the command.
```

### `logs`

View sandbox logs. This is a top-level command (alias: `lg`).

```bash
openshellctl logs my-sandbox
openshellctl logs my-sandbox --tail            # stream live
openshellctl logs my-sandbox -n 50             # last 50 lines
openshellctl logs my-sandbox --since 5m        # last 5 minutes
openshellctl logs my-sandbox --level error     # errors only
openshellctl logs my-sandbox --source gateway  # gateway logs only
```

| Flag | Description |
|------|-------------|
| `--tail` | Stream live logs |
| `-n` | Number of log lines (default: 200) |
| `--since` | Show logs from this duration ago (e.g. `5m`, `1h`, `30s`) |
| `--level` | Minimum level: `error`, `warn`, `info`, `debug`, `trace` |
| `--source` | Filter by source: `gateway`, `sandbox`, or `all` (default) |

### `token`

Inspect and manage the gateway authentication token.

```bash
openshellctl token show                # show token metadata
openshellctl token refresh             # force refresh
openshellctl token refresh --write     # refresh and persist
openshellctl token inspect <JWT>       # decode JWT claims
```

### `policy lint`

Client-side validation of a sandbox policy file.

```bash
openshellctl policy lint policy.yaml
```

Checks cover process uid/gid ranges, filesystem path constraints, middleware selectors, endpoint ports, and TCP access rules. The gateway's server-side validation remains authoritative.

### `version`

Print build information.

```bash
openshellctl version
```

Shows: version, commit, OpenShell pin version, and SDK version.

### `completion`

Generate shell completion scripts.

```bash
openshellctl completion bash
openshellctl completion zsh
openshellctl completion fish
openshellctl completion powershell
```

## Aliases

| Command | Alias |
|---------|-------|
| `sandbox` | `sb` |
| `logs` | `lg` |

```bash
openshellctl sb list          # same as: openshellctl sandbox list
openshellctl lg my-sandbox    # same as: openshellctl logs my-sandbox
```

## Last-used sandbox

Commands that take a sandbox name remember the last-used sandbox per gateway and workspace. If you omit the name argument, the last-used sandbox is used automatically:

```bash
openshellctl sandbox exec --name my-sandbox -- whoami   # saves my-sandbox
openshellctl sandbox exec -- whoami                     # reuses my-sandbox
openshellctl sandbox connect                            # reuses my-sandbox
openshellctl logs                                       # reuses my-sandbox
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic / RPC error |
| 2 | Usage error (bad flags, validation failure, invalid argument rejected by gateway) |
| 3 | Authentication failure (expired/invalid token, missing credentials) |
| 4 | Not found (sandbox, gateway) |
| 5 | Conflict (already exists, concurrent modification) |
| 6 | Provisioning failure or timeout |
| N | Remote process exit code (from `exec`/`connect`/`create` with attached command) |

## Environment variables

| Variable | Description |
|----------|-------------|
| `OPENSHELL_GATEWAY` | Default gateway name |
| `OPENSHELL_GATEWAY_ENDPOINT` | Gateway endpoint URL |
| `OPENSHELL_GATEWAY_INSECURE` | Skip TLS verification (`true`/`1`) |
| `OPENSHELL_WORKSPACE` | Default workspace |
| `OPENSHELL_TOKEN` | Bearer token |
| `OPENSHELL_SANDBOX_POLICY` | Default sandbox policy file path |
| `NO_COLOR` | Disable coloured output (any value) |

## Differences from the upstream Rust CLI

openshellctl is a compatible reimplementation with these intentional differences:

- **Pure Go**: no shell-out to `ssh`, `tar`, `git`, or any other binary. All SSH transfer, tar streaming, and gitignore filtering are implemented in-process.
- **Embeddable**: `pkg/gateway`, `pkg/transfer`, `pkg/sandbox`, and `pkg/auth` are importable Go packages with interface-driven I/O seams, suitable for use in operators and controllers.
- **No interactive spinner**: provisioning progress uses plain text output instead of terminal spinners.
- **`policy lint` is a separate top-level command** (not under `sandbox`).
- **Gitignore filtering**: uses compiled `.gitignore` rules instead of shelling out to `git ls-files`. Tracked-but-ignored files are excluded (matching untracked behaviour) rather than re-included.

## License

See [LICENSE](LICENSE).
