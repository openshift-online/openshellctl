package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// fencedBlockRe matches a fenced Markdown code block (``` ... ```), any or
// no language tag, non-greedy so adjacent blocks in the same file don't
// merge into one match.
var fencedBlockRe = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

// extractOpenshellctlLines scans every fenced code block in markdown and
// returns each logical line whose first token is "openshellctl" — a
// transcript's output lines (e.g. a `doctor` table's "✓ Endpoint URL ..."
// rows, or "$ echo $?" / "3") never start with that token and are never
// selected. Backslash line continuations are joined into one logical line
// first; a leading "$ " shell-prompt marker and a trailing "# comment" are
// stripped.
func extractOpenshellctlLines(markdown string) []string {
	var out []string
	for _, block := range fencedBlockRe.FindAllStringSubmatch(markdown, -1) {
		out = append(out, linesFromBlock(block[1])...)
	}
	return out
}

// linesFromBlock joins backslash-continued lines within one fenced block,
// then returns the cleaned "openshellctl ..." lines from it.
func linesFromBlock(block string) []string {
	var out []string
	var pending string
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimRight(raw, " \t")
		if pending != "" {
			line = pending + " " + strings.TrimLeft(line, " \t")
			pending = ""
		}
		if strings.HasSuffix(line, "\\") {
			pending = strings.TrimRight(strings.TrimSuffix(line, "\\"), " \t")
			continue
		}
		line = cleanExampleLine(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "openshellctl" {
			out = append(out, line)
		}
	}
	// A trailing unterminated continuation (malformed example) is dropped,
	// not silently merged with whatever follows the fence.
	return out
}

// cleanExampleLine strips a leading "$ " shell-prompt marker and a trailing
// "# comment" (only outside of quotes, so a literal "#" in an argument
// value — none of today's examples have one, but a future one might — is
// never mistaken for a comment).
func cleanExampleLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "$ ")
	inSingle, inDouble := false, false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (i == 0 || line[i-1] == ' ') {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return strings.TrimSpace(line)
}

// shellSplit is a minimal quote-aware tokenizer — single/double quotes
// group a token, backslash escapes the next character — sufficient for
// these docs' actual argument shapes (plain words, "<placeholder>" values,
// quoted env-var interpolations like "${NAME}"). It is not a full shell
// grammar (no globbing, no command substitution, no variable expansion);
// these examples never need either.
func shellSplit(line string) []string {
	var tokens []string
	var cur strings.Builder
	var inSingle, inDouble, haveToken bool
	flush := func() {
		if haveToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			haveToken = false
		}
	}
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\' && !inSingle && i+1 < len(runes):
			i++
			cur.WriteRune(runes[i])
			haveToken = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			haveToken = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			haveToken = true
		case r == ' ' || r == '\t':
			if inSingle || inDouble {
				cur.WriteRune(r)
			} else {
				flush()
			}
		default:
			cur.WriteRune(r)
			haveToken = true
		}
	}
	flush()
	return tokens
}

// TestDocsExamplesParse is ROSAENG-68833's acceptance criterion: every
// fenced `openshellctl ...` line in README.md and docs/*.md must parse
// against the real cobra command tree. "Parse" means Command.Traverse (the
// persistent-flag-aware tree walk) resolves a command and ParseFlags
// accepts the remainder — RunE is never called, so this is fully hermetic
// (no network, no gateway dial) even for examples that would otherwise
// require a live gateway.
func TestDocsExamplesParse(t *testing.T) {
	paths := docSourcePaths(t)
	if len(paths) == 0 {
		t.Fatal("no doc sources found — README.md/docs/*.md glob is broken")
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		lines := extractOpenshellctlLines(string(data))
		for i, line := range lines {
			name := fmt.Sprintf("%s#%d", filepath.Base(path), i+1)
			t.Run(name, func(t *testing.T) {
				viper.Reset()
				t.Cleanup(viper.Reset)

				tokens := shellSplit(line)
				if len(tokens) == 0 || tokens[0] != "openshellctl" {
					t.Fatalf("not an openshellctl invocation: %q", line)
				}
				args := tokens[1:]

				root := NewRootCommand()
				cmd, remaining, err := root.Traverse(args)
				if err != nil {
					t.Fatalf("Traverse(%q) (from %s): %v", line, path, err)
				}
				if err := cmd.ParseFlags(remaining); err != nil {
					t.Fatalf("ParseFlags(%q) (from %s): %v", line, path, err)
				}
			})
		}
	}
}

// docSourcePaths returns README.md plus every docs/*.md file, relative to
// the internal/cli package's own directory (../../ reaches the repo root).
func docSourcePaths(t *testing.T) []string {
	t.Helper()
	paths := []string{"../../README.md"}
	matches, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatalf("globbing docs/*.md: %v", err)
	}
	return append(paths, matches...)
}

func TestExtractOpenshellctlLines(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []string
	}{
		{
			name:     "no fenced block",
			markdown: "just some prose about openshellctl, no code fence",
			want:     nil,
		},
		{
			name:     "simple invocation",
			markdown: "```bash\nopenshellctl sandbox list\n```",
			want:     []string{"openshellctl sandbox list"},
		},
		{
			name:     "leading dollar prompt stripped",
			markdown: "```\n$ openshellctl doctor\n```",
			want:     []string{"openshellctl doctor"},
		},
		{
			name: "transcript output lines are not selected",
			markdown: "```\n$ openshellctl doctor\n" +
				"✓ Endpoint URL       http://gw.example.com\n" +
				"$ echo $?\n3\n```",
			want: []string{"openshellctl doctor"},
		},
		{
			name: "backslash continuation joined into one logical line",
			markdown: "```bash\nopenshellctl sandbox create \\\n" +
				"  --name foo \\\n  --from python\n```",
			want: []string{"openshellctl sandbox create --name foo --from python"},
		},
		{
			name:     "trailing comment stripped",
			markdown: "```bash\nopenshellctl sandbox list # list everything\n```",
			want:     []string{"openshellctl sandbox list"},
		},
		{
			name:     "no language tag on the fence still matches",
			markdown: "```\nopenshellctl version\n```",
			want:     []string{"openshellctl version"},
		},
		{
			name: "two separate fenced blocks",
			markdown: "```bash\nopenshellctl gateway add <endpoint>\n```\n" +
				"some prose\n```bash\nopenshellctl gateway login\n```",
			want: []string{"openshellctl gateway add <endpoint>", "openshellctl gateway login"},
		},
		{
			name:     "a non-openshellctl command in a fence is ignored",
			markdown: "```bash\nexport OPENSHELL_TOKEN=$(cat token)\n```",
			want:     nil,
		},
		{
			name:     "env var export line is ignored even with an openshellctl-looking value",
			markdown: "```bash\nVAR=openshellctl-is-not-exported\n```",
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOpenshellctlLines(tt.markdown)
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("extractOpenshellctlLines(%q) = %#v, want %#v", tt.markdown, got, tt.want)
			}
		})
	}
}

func TestShellSplit(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{"plain words", "sandbox create --from python", []string{"sandbox", "create", "--from", "python"}},
		{"double-quoted value with spaces", `sandbox create --name "my sandbox"`, []string{"sandbox", "create", "--name", "my sandbox"}},
		{"single-quoted value", `gateway add https://gw --name 'my gw'`, []string{"gateway", "add", "https://gw", "--name", "my gw"}},
		{"env interpolation placeholder stays literal", `sandbox create --name "${NAME}"`, []string{"sandbox", "create", "--name", "${NAME}"}},
		{"backslash-escaped space", `sandbox create --name foo\ bar`, []string{"sandbox", "create", "--name", "foo bar"}},
		{"empty line", "", nil},
		{"placeholder angle brackets", "gateway add <endpoint> --name <name>", []string{"gateway", "add", "<endpoint>", "--name", "<name>"}},
		{"trailing double-dash passthrough", `sandbox create --name x -- claude --print "hi"`, []string{"sandbox", "create", "--name", "x", "--", "claude", "--print", "hi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellSplit(tt.line)
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("shellSplit(%q) = %#v, want %#v", tt.line, got, tt.want)
			}
		})
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
