package paramtemplate

import (
	"reflect"
	"strings"
	"testing"
)

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		values  map[string]string
		want    string
		missing []string
	}{
		{
			name:   "no references, document unchanged",
			yaml:   "metadata:\n  name: fixed\n",
			values: nil,
			want:   "metadata:\n    name: fixed\n",
		},
		{
			name:   "simple substitution",
			yaml:   "metadata:\n  name: ${NAME}\n",
			values: map[string]string{"NAME": "myname"},
			want:   "metadata:\n    name: myname\n",
		},
		{
			name:   "value with comma and colon is sent through exactly, never re-parsed as YAML",
			yaml:   "spec:\n  command:\n    - '${PROMPT}'\n",
			values: map[string]string{"PROMPT": "a, b: c"},
			want:   "spec:\n    command:\n        - 'a, b: c'\n",
		},
		{
			name:   "multiple references embedded in literal text",
			yaml:   "metadata:\n  name: ${A}-${B}\n",
			values: map[string]string{"A": "x", "B": "y"},
			want:   "metadata:\n    name: x-y\n",
		},
		{
			name:   "escaped $${KEY} is left untouched",
			yaml:   "metadata:\n  name: $${ESCAPED}\n",
			values: map[string]string{"ESCAPED": "should-not-appear"},
			want:   "metadata:\n    name: $${ESCAPED}\n",
		},
		{
			name:   "bare $KEY without braces is left untouched",
			yaml:   "metadata:\n  name: $NOTAREF\n",
			values: map[string]string{"NOTAREF": "should-not-appear"},
			want:   "metadata:\n    name: $NOTAREF\n",
		},
		{
			name:    "unresolved reference is reported, not substituted",
			yaml:    "metadata:\n  name: ${NAME}\n",
			values:  map[string]string{},
			want:    "metadata:\n    name: ${NAME}\n",
			missing: []string{"NAME"},
		},
		{
			name:    "missing keys are sorted and deduplicated across the document",
			yaml:    "metadata:\n  name: ${B}\nspec:\n  image: ${A}\n  env:\n    X: ${B}\n",
			values:  map[string]string{},
			missing: []string{"A", "B"},
		},
		{
			name:   "map keys are never substituted into",
			yaml:   "spec:\n  env:\n    '${NAME}': value\n",
			values: map[string]string{"NAME": "should-not-replace-key"},
			want:   "spec:\n    env:\n        '${NAME}': value\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseDocument([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			missing := Substitute(doc, tt.values)
			if !reflect.DeepEqual(missing, tt.missing) {
				t.Errorf("missing = %#v, want %#v", missing, tt.missing)
			}
			if tt.want == "" {
				return
			}
			got, err := Render(doc)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubstitute_RoundTripsThroughDecode(t *testing.T) {
	// Confirms the design claim in the package doc: a value containing a
	// comma, a colon and an embedded "=" survives Render byte-for-byte as a
	// Go string, because substitution happens on the parsed node's decoded
	// Value, never on raw file text.
	doc, err := ParseDocument([]byte("spec:\n  command:\n    - claude\n    - '${PROMPT}'\n"))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	missing := Substitute(doc, map[string]string{"PROMPT": "a, b: c=d"})
	if len(missing) != 0 {
		t.Fatalf("unexpected missing: %v", missing)
	}
	out, err := Render(doc)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "a, b: c=d") {
		t.Errorf("rendered output does not contain the exact substituted value:\n%s", out)
	}
}
