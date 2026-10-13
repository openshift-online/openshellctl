package paramtemplate

import "strings"

// ParseParamFileContent parses a --param-file's content: one KEY=VALUE per
// line, '#'-prefixed lines and blank/whitespace-only lines ignored. path
// identifies the file in any resulting *MalformedParamError. A later
// duplicate key within the same file overwrites an earlier one.
func ParseParamFileContent(path string, data []byte) (map[string]string, error) {
	out := map[string]string{}
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, err := parseParamEntry(line, path, i+1)
		if err != nil {
			return nil, err
		}
		out[p.Key] = p.Value
	}
	return out, nil
}
