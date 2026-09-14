// Package sandbox holds pure helper functions (image/quantity/env/label/provider
// parsing, spec construction) and the orchestration of sandbox lifecycle
// operations over a gateway.Gateway. Pure functions mirror the upstream CLI
// behaviour byte-for-byte (spec §5.5, Appendix A).
package sandbox

import (
	"io/fs"
	"strings"
)

// DefaultCommunityRegistry is the default registry for bare community image names.
const DefaultCommunityRegistry = "ghcr.io/nvidia/openshell-community/sandboxes"

const localBuildUnsupportedMsg = "local image builds are not supported by openshellctl; build and push the image, then pass an image reference to --from"

// ErrLocalBuildUnsupported reports a Dockerfile/dir-with-Dockerfile --from, which
// openshellctl deliberately does not support (it cannot build/push).
type ErrLocalBuildUnsupported struct{ Path string }

func (e *ErrLocalBuildUnsupported) Error() string { return localBuildUnsupportedMsg }

// ErrNoDockerfile reports a directory --from with no Dockerfile.
type ErrNoDockerfile struct{ Path string }

func (e *ErrNoDockerfile) Error() string { return "No Dockerfile found" }

// ErrLocalPathMissing reports a --from that looks like a local path but does not exist.
type ErrLocalPathMissing struct{ Path string }

func (e *ErrLocalPathMissing) Error() string {
	return "local path does not exist: " + e.Path
}

// ResolveImage maps a --from value to an image reference, mirroring run.rs
// resolve_from (run.rs:1046-1117). stat is injected for testability.
//
// Rules in order:
//  1. stat ok and a file whose lowercased base contains "dockerfile" or ends
//     ".dockerfile" → ErrLocalBuildUnsupported;
//  2. stat ok and a dir containing "Dockerfile" → ErrLocalBuildUnsupported;
//     a dir without → ErrNoDockerfile;
//  3. looks like a local path and does not exist → ErrLocalPathMissing;
//  4. contains '/', ':' or '.' → verbatim image ref;
//  5. else → <registry-or-default>/<from>:latest.
func ResolveImage(from, registry string, stat func(string) (fs.FileInfo, error)) (string, error) {
	if registry == "" {
		registry = DefaultCommunityRegistry
	}

	info, statErr := stat(from)
	if statErr == nil {
		if info.IsDir() {
			if _, err := stat(joinPath(from, "Dockerfile")); err == nil {
				return "", &ErrLocalBuildUnsupported{Path: from}
			}
			return "", &ErrNoDockerfile{Path: from}
		}
		if looksLikeDockerfile(baseName(from)) {
			return "", &ErrLocalBuildUnsupported{Path: from}
		}
		// A stat-able regular file that is not a Dockerfile is treated as a
		// local path that cannot be an image (fall through to path handling).
	}

	if looksLikeLocalPath(from) {
		return "", &ErrLocalPathMissing{Path: from}
	}

	if strings.ContainsAny(from, "/:.") {
		return from, nil
	}

	return registry + "/" + from + ":latest", nil
}

func looksLikeDockerfile(base string) bool {
	lb := strings.ToLower(base)
	return strings.Contains(lb, "dockerfile") || strings.HasSuffix(lb, ".dockerfile")
}

// looksLikeLocalPath reports whether from is shaped like a filesystem path:
// absolute, ".", "..", or prefixed with "./", "../", "~/", or a bare name that
// contains "dockerfile" with no '/' or ':'.
func looksLikeLocalPath(from string) bool {
	switch {
	case from == "." || from == "..":
		return true
	case strings.HasPrefix(from, "/"),
		strings.HasPrefix(from, "./"),
		strings.HasPrefix(from, "../"),
		strings.HasPrefix(from, "~/"):
		return true
	case strings.Contains(strings.ToLower(from), "dockerfile") &&
		!strings.ContainsAny(from, "/:"):
		return true
	default:
		return false
	}
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func joinPath(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}
