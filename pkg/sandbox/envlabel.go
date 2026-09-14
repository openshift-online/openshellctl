package sandbox

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ParseEnvPairs parses --env KEY=VALUE items (common.rs:842-876). Split on the
// first '='; the key is trimmed; the value is untouched. Messages are verbatim.
func ParseEnvPairs(items []string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range items {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("--env expects KEY=VALUE, got '%s'", item)
		}
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, fmt.Errorf("--env key cannot be empty")
		}
		if !envKeyRe.MatchString(k) {
			return nil, fmt.Errorf("--env key must match [A-Za-z_][A-Za-z0-9_]*; got '%s'", k)
		}
		if strings.HasPrefix(k, "OPENSHELL_") {
			return nil, fmt.Errorf("--env keys starting with OPENSHELL_ are reserved; got '%s'", k)
		}
		out[k] = v
	}
	return out, nil
}

// ParseLabels parses --label key=value items (main.rs:3029-3040). Split on the
// first '='; no trimming. Message verbatim.
func ParseLabels(items []string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range items {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("invalid label format '%s', expected key=value", item)
		}
		out[k] = v
	}
	return out, nil
}

// credentialKeywords are the segment-window keywords for the credential
// heuristic (common.rs:758-840). Each entry is a sequence of segments that must
// appear consecutively in the uppercased, '_'-split key.
var credentialKeywords = [][]string{
	{"TOKEN"},
	{"SECRET"},
	{"PASSWORD"},
	{"CREDENTIAL"},
	{"ACCESS", "KEY"},
	{"SECRET", "KEY"},
	{"API", "KEY"},
}

// CredentialWarning identifies an env key that looks like a credential.
type CredentialWarning struct {
	Key         string
	Suggestions []CredentialSuggestion
}

// CredentialSuggestion is a provider-based alternative for a known credential var.
type CredentialSuggestion struct {
	Type string // e.g. "github"
	Key  string // the env var name
}

// CredentialLikeKeys returns credential-looking env keys, sorted by key. It
// matches keyword segment-windows (MY_ACCESS_KEY yes; PRIMARY_KEY,
// TOKENIZERS_PARALLELISM no) plus known builtin-profile credential var names.
func CredentialLikeKeys(env map[string]string) []CredentialWarning {
	var out []CredentialWarning
	for k := range env {
		if sugg, ok := builtinProfileCredentials[strings.ToUpper(k)]; ok {
			out = append(out, CredentialWarning{Key: k, Suggestions: sugg})
			continue
		}
		if matchesCredentialKeyword(k) {
			out = append(out, CredentialWarning{Key: k})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// matchesCredentialKeyword reports whether the uppercased '_'-split key contains
// any credential keyword as a consecutive segment window.
func matchesCredentialKeyword(key string) bool {
	segs := strings.Split(strings.ToUpper(key), "_")
	for _, kw := range credentialKeywords {
		if containsWindow(segs, kw) {
			return true
		}
	}
	return false
}

func containsWindow(segs, window []string) bool {
	if len(window) == 0 || len(window) > len(segs) {
		return false
	}
	for i := 0; i+len(window) <= len(segs); i++ {
		match := true
		for j, w := range window {
			if segs[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// FormatCredentialWarning renders the stderr warning block (Appendix A.5).
func FormatCredentialWarning(w CredentialWarning) string {
	var b strings.Builder
	fmt.Fprintf(&b, "⚠ %s looks like a credential passed as a plain environment variable.\n", w.Key)
	b.WriteString("  The agent inside the sandbox can read this value directly.\n\n")
	if len(w.Suggestions) == 0 {
		b.WriteString("  To hide it from the agent, use a provider instead of --env.\n")
		return b.String()
	}
	b.WriteString("  To hide it from the agent, use a provider instead:\n")
	for _, s := range w.Suggestions {
		fmt.Fprintf(&b, "    openshell provider create --name my-%s --type %s --credential %s\n", s.Type, s.Type, w.Key)
		fmt.Fprintf(&b, "    openshell sandbox create --provider my-%s ...\n", s.Type)
	}
	b.WriteString("  See: https://docs.nvidia.com/openshell/latest/sandboxes/providers-v2\n\n")
	return b.String()
}
