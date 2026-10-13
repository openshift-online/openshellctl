package paramtemplate

import (
	"errors"
	"testing"
)

func TestParseParam(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Param
		wantErr bool
	}{
		{"simple", "NAME=X", Param{Key: "NAME", Value: "X"}, false},
		{"value contains comma", "PROMPT=a, b: c", Param{Key: "PROMPT", Value: "a, b: c"}, false},
		{"value contains equals", "A=b=c", Param{Key: "A", Value: "b=c"}, false},
		{"empty value is allowed", "A=", Param{Key: "A", Value: ""}, false},
		{"underscore and digits in key", "A_1=x", Param{Key: "A_1", Value: "x"}, false},
		{"leading underscore key", "_A=x", Param{Key: "_A", Value: "x"}, false},

		{"no equals sign", "NAME", Param{}, true},
		{"empty key", "=x", Param{}, true},
		{"key starts with digit", "1A=x", Param{}, true},
		{"key contains hyphen", "A-B=x", Param{}, true},
		{"key contains space", "A B=x", Param{}, true},
		{"empty raw", "", Param{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseParam(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseParam(%q) = %+v, nil; want error", tt.raw, got)
				}
				var malformed *MalformedParamError
				if !errors.As(err, &malformed) {
					t.Errorf("ParseParam(%q) error type = %T, want *MalformedParamError", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseParam(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ParseParam(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMalformedParamError_Message(t *testing.T) {
	err := &MalformedParamError{Raw: "NAME", Source: "--param"}
	got := err.Error()
	want := `--param: invalid parameter "NAME", expected KEY=VALUE with KEY matching [A-Za-z_][A-Za-z0-9_]*`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	errLine := &MalformedParamError{Raw: "NAME", Source: "params.txt", Line: 3}
	gotLine := errLine.Error()
	wantLine := `params.txt:3: invalid parameter "NAME", expected KEY=VALUE with KEY matching [A-Za-z_][A-Za-z0-9_]*`
	if gotLine != wantLine {
		t.Errorf("Error() = %q, want %q", gotLine, wantLine)
	}
}
