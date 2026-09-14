package transfer

import "strings"

// lexicalCleanAbsolutePath cleans an absolute POSIX path without touching the
// filesystem: it must start with "/", resolves "." and "..", collapses "//",
// and strips a trailing "/". An empty result is "/". Mirrors ssh.rs:840-863.
// ok is false when the path is not absolute.
func lexicalCleanAbsolutePath(p string) (string, bool) {
	if !strings.HasPrefix(p, "/") {
		return "", false
	}
	var stack []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".":
			// skip empty (collapses //) and current-dir segments
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, seg)
		}
	}
	if len(stack) == 0 {
		return "/", true
	}
	return "/" + strings.Join(stack, "/"), true
}

// pathIsOrUnder reports whether p equals root or is nested under it. Mirrors
// path_is_or_under (driver_mounts.rs:219-221).
func pathIsOrUnder(p, root string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}
