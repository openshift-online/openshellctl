package transfer

import "strings"

// ShellEscape mirrors forward.rs:782-796. An empty string becomes ''. A string
// whose every byte is ASCII-alphanumeric or one of . / - _ is returned as-is;
// otherwise it is single-quote wrapped with embedded ' replaced by '”'”'.
func ShellEscape(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for i := 0; i < len(s); i++ {
		if !shellSafeByte(s[i]) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func shellSafeByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '.', b == '/', b == '-', b == '_':
		return true
	default:
		return false
	}
}

// splitSandboxPath mirrors ssh.rs:822-828: rfind '/'; pos 0 → ("/", rest);
// other → (prefix, suffix); none → (".", path).
func splitSandboxPath(p string) (parent, name string) {
	i := strings.LastIndexByte(p, '/')
	switch {
	case i < 0:
		return ".", p
	case i == 0:
		return "/", p[1:]
	default:
		return p[:i], p[i+1:]
	}
}
