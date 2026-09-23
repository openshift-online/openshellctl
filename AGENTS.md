# openshellctl

Lightweight Go CLI and embeddable client for NVIDIA OpenShell gateways.
Pure-Go reimplementation of the upstream Rust CLI — no shell-out to ssh, tar, git, or any external binary.

## Build and test

```bash
make build              # build via goreleaser (snapshot)
make test               # unit tests
make lint               # golangci-lint
make test-race          # tests with race detector
make coverage           # coverage report
make test-coverage-threshold  # enforce coverage minimums
make verify             # pin + tidy + generate checks
make test-all           # all of the above
```

Go version is declared in `go.mod` (currently 1.26). CI runs on GitHub Actions (`.github/workflows/ci.yml`): verify, test, lint, image build.

## Project layout

```
cmd/openshellctl/       # main entrypoint
internal/cli/           # cobra command tree, CLI integration tests
pkg/api/v1alpha1/       # API types (Sandbox manifest schema)
pkg/auth/               # OIDC token resolution and refresh
pkg/gateway/            # gRPC gateway client (interface + impl)
pkg/gateway/mock/       # generated mock for gateway.Gateway
pkg/gatewayconfig/      # config file and env resolution
pkg/output/             # table/json/yaml output formatting
pkg/policyyaml/         # sandbox policy parsing and validation
pkg/sandbox/            # sandbox spec merging and construction
pkg/transfer/           # SSH file transfer (upload/download/connect/exec)
internal/version/       # build metadata (version, commit, pin)
docs/plans/             # design docs and implementation plan
hack/                   # verification scripts, pins
hack/parity/            # upstream CLI parity tracking
build/                  # Dockerfile
```

## Code architecture

The codebase follows a **main-driver + pure-functions** pattern:

- **Thin entry points** (`internal/cli/`) wire cobra commands to business logic.
- **Pure functions** contain all decisions and transformations — they take values in and return values out with no side effects. Examples: `sanitizeTarName`, `validateSymlinkTarget`, `parseAndValidateSourcePath`, `planUpload`, `shellEscape`.
- **Thin I/O shells** call the pure functions and handle the actual SSH/gRPC/filesystem interaction. The shells are deliberately small so they don't need complex mocking.
- **Interfaces at boundaries**: `gateway.Gateway` is the primary seam — all CLI tests inject a mock gateway. `fs.FS` with extension interfaces (`LstatFS`, `ReadlinkFS`) abstracts filesystem access for upload logic.

When adding new functionality, follow this pattern: extract every decision into a pure function, test it exhaustively, and keep the I/O shell minimal.

## Development workflow

Follow test-driven development:

1. **Write tests first** based on the desired behavior and edge cases.
2. **Implement the code** to make the tests pass.
3. **Validate** by running `make test` and `make lint`.

## Testing requirements

### Coverage

Coverage thresholds are enforced in CI via `make test-coverage-threshold`:
- `pkg/` packages: minimum 85%
- `internal/cli/`: minimum 70%

These thresholds are a **floor, not a target**. Aim to cover every input/output path for a given function. Do not test imported libraries or standard-library behavior — test your code's logic.

### Hermetic tests

All unit tests must be fully hermetic:

- **No network calls**: no HTTP, gRPC, or API requests. Mock all external services via interfaces (e.g., `gateway.Gateway` mock).
- **No host filesystem access**: use `t.TempDir()` for any filesystem operations. Never read from or write to the real filesystem outside the temp directory.
- **No external tooling**: do not shell out to `ssh`, `tar`, `git`, `sh`, or any other binary. All such functionality is implemented in-process (archive/tar, golang.org/x/crypto/ssh, go-gitignore).
- **No host state**: tests must not depend on environment variables, user config, running services, or network availability.

The test SSH server runs in-process over buffered in-memory connections, authenticates with `NoClientAuth`, and implements remote commands in Go against `t.TempDir()` trees.

### Test structure

- Use **table-driven tests** with named subtests (`t.Run`).
- Use `t.Helper()` in test helper functions.
- Test both valid and invalid inputs — every error path a function can produce should have a corresponding test case.
- Place pure-function tests in `*_test.go` alongside the code. The `pure_test.go` convention groups all pure-logic tests in a package; `smoke_test.go` covers I/O integration.

### What to test vs. what not to test

**Test**: every pure function's input/output paths — valid inputs, boundary cases, and each distinct error condition your code produces.

**Do not test**: behavior of imported libraries (e.g., don't test that `archive/tar` correctly reads tar headers, or that `os.OpenRoot` rejects escapes — test that *your code* calls them correctly and handles their return values). Do not test Go standard library contracts.

## Style and conventions

- Go source follows `gofumpt` formatting (enforced by golangci-lint).
- Prefer unexported functions unless the API surface requires export.
- No CGO — the binary is statically linked (`CGO_ENABLED=0`).
- Container engine is `podman` (not docker) for local development; CI uses `docker`.
- Binary output goes to `/tmp` for manual builds; use `make build` when possible.
