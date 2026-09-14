package policyyaml

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// QueryMatcher is a glob string OR {any: [...]} (lib.rs:341, untagged). On
// decode: a JSON string → Glob; a JSON object is strictly decoded as {any:[...]}
// (unknown keys rejected). IsAny distinguishes the two forms.
type QueryMatcher struct {
	IsAny bool
	Glob  string
	Any   []string
}

// queryAny is the strict {any: [...]} shape (QueryAnyDef, deny_unknown_fields).
type queryAny struct {
	Any []string `json:"any"`
}

// UnmarshalJSON implements the untagged QueryMatcherDef decode.
func (m *QueryMatcher) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return errors.New("data did not match any variant of untagged enum QueryMatcherDef")
	}
	switch data[0] {
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		m.IsAny = false
		m.Glob = s
		return nil
	case '{':
		var qa queryAny
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&qa); err != nil {
			return errors.New("data did not match any variant of untagged enum QueryMatcherDef")
		}
		m.IsAny = true
		m.Any = qa.Any
		return nil
	default:
		return errors.New("data did not match any variant of untagged enum QueryMatcherDef")
	}
}

// MarshalJSON emits the glob string or {"any":[...]}.
func (m QueryMatcher) MarshalJSON() ([]byte, error) {
	if m.IsAny {
		return json.Marshal(queryAny{Any: m.Any})
	}
	return json.Marshal(m.Glob)
}

// ParamMatcher is a QueryMatcher OR a nested object of ParamMatchers (lib.rs:352,
// untagged). Decode tries the matcher form first (string or strict {any});
// on failure a mapping is a nested object.
type ParamMatcher struct {
	Matcher *QueryMatcher
	Object  map[string]ParamMatcher
}

// UnmarshalJSON implements the untagged ParamMatcherDef decode.
func (p *ParamMatcher) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return errors.New("data did not match any variant of untagged enum ParamMatcherDef")
	}
	switch data[0] {
	case '"':
		var qm QueryMatcher
		if err := qm.UnmarshalJSON(data); err != nil {
			return err
		}
		p.Matcher = &qm
		return nil
	case '{':
		// Try the matcher form first: strict {any:[...]}.
		var qm QueryMatcher
		if err := qm.UnmarshalJSON(data); err == nil {
			p.Matcher = &qm
			return nil
		}
		// Otherwise a nested object of ParamMatchers.
		var obj map[string]ParamMatcher
		if err := json.Unmarshal(data, &obj); err != nil {
			return errors.New("data did not match any variant of untagged enum ParamMatcherDef")
		}
		p.Object = obj
		return nil
	default:
		return errors.New("data did not match any variant of untagged enum ParamMatcherDef")
	}
}

// MarshalJSON emits the matcher or the nested object.
func (p ParamMatcher) MarshalJSON() ([]byte, error) {
	if p.Matcher != nil {
		return p.Matcher.MarshalJSON()
	}
	if p.Object != nil {
		return json.Marshal(p.Object)
	}
	return nil, fmt.Errorf("empty ParamMatcher")
}
