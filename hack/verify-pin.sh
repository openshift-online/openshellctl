#!/usr/bin/env bash
# verify-pin.sh — assert that hack/openshell-pin, the upstream tag's commit, and
# the SDK pseudo-version in go.mod all agree. Fails loudly on drift (spec §4).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pin_file="${repo_root}/hack/openshell-pin"
go_mod="${repo_root}/go.mod"

if [[ ! -f "${pin_file}" ]]; then
  echo "verify-pin: ${pin_file} not found" >&2
  exit 1
fi

read -r pin_tag pin_commit < "${pin_file}"
if [[ -z "${pin_tag}" || -z "${pin_commit}" ]]; then
  echo "verify-pin: hack/openshell-pin must contain '<tag> <commit>'" >&2
  exit 1
fi

echo "verify-pin: pinned to ${pin_tag} (${pin_commit})"

# 0. The compiled-in default pin must match the pin file.
version_go="${repo_root}/internal/version/version.go"
if ! grep -q "defaultOpenShellPin = \"${pin_tag}\"" "${version_go}"; then
  echo "verify-pin: internal/version/version.go defaultOpenShellPin != ${pin_tag}" >&2
  grep "defaultOpenShellPin" "${version_go}" >&2 || true
  exit 1
fi
echo "verify-pin: source default pin matches ${pin_tag} — OK"

# 1. When the SDK is a dependency, its go.mod pseudo-version must embed the
#    pinned commit's short hash. The SDK enters go.mod once the first package
#    imports it; until then this cross-check is skipped (the pin file remains the
#    single source of truth for the `version` command and the image build).
short_commit="${pin_commit:0:12}"
if grep -q "NVIDIA/OpenShell/sdk/go" "${go_mod}"; then
  if ! grep -q "NVIDIA/OpenShell/sdk/go .*${short_commit}" "${go_mod}"; then
    echo "verify-pin: go.mod SDK pseudo-version does not embed commit ${short_commit}" >&2
    grep "NVIDIA/OpenShell/sdk/go" "${go_mod}" >&2 || true
    exit 1
  fi
  echo "verify-pin: go.mod pseudo-version embeds ${short_commit} — OK"
else
  echo "verify-pin: SDK not yet a go.mod dependency — skipping pseudo-version cross-check"
fi

# 2. When network is available, confirm the tag still resolves to the pinned commit.
if [[ "${VERIFY_PIN_OFFLINE:-0}" != "1" ]]; then
  remote_commit="$(git ls-remote https://github.com/NVIDIA/OpenShell "refs/tags/${pin_tag}^{}" 2>/dev/null | awk '{print $1}' || true)"
  if [[ -z "${remote_commit}" ]]; then
    # Fall back to the lightweight (non-annotated) ref.
    remote_commit="$(git ls-remote https://github.com/NVIDIA/OpenShell "refs/tags/${pin_tag}" 2>/dev/null | awk '{print $1}' || true)"
  fi
  if [[ -z "${remote_commit}" ]]; then
    echo "verify-pin: could not reach upstream to confirm the tag (set VERIFY_PIN_OFFLINE=1 to skip)" >&2
    exit 1
  fi
  if [[ "${remote_commit}" != "${pin_commit}" ]]; then
    echo "verify-pin: tag ${pin_tag} now points at ${remote_commit}, not ${pin_commit}" >&2
    exit 1
  fi
  echo "verify-pin: upstream tag ${pin_tag} still resolves to ${pin_commit} — OK"
else
  echo "verify-pin: VERIFY_PIN_OFFLINE=1 set — skipping upstream tag check"
fi

echo "verify-pin: OK"
