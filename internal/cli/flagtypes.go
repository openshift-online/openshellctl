package cli

import "strconv"

// ttyTriState records the effective value of the --tty / --no-tty pair under
// clap `overrides_with` (last-wins) semantics: whichever flag is parsed last
// sets the value. nil (unset) means auto-detect. Because pflag parses flags
// left-to-right, "last-wins" falls out naturally from each Set call overwriting
// the previous.
type ttyTriState struct {
	set bool
	val bool
}

// Value returns a pointer to the effective bool, or nil when neither flag was
// given.
func (t *ttyTriState) Value() *bool {
	if !t.set {
		return nil
	}
	v := t.val
	return &v
}

// ttyStateFlag is a pflag.Value bound to one spelling of the tty flag. `positive`
// is true for --tty and false for --no-tty; parsing the flag with value "true"
// records the corresponding effective value into the shared ttyTriState.
type ttyStateFlag struct {
	state    *ttyTriState
	positive bool
}

func (f ttyStateFlag) String() string { return "" }
func (f ttyStateFlag) Type() string   { return "ttyTriState" }

func (f ttyStateFlag) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	f.state.set = true
	// --tty means force-on, --no-tty means force-off. `b` is the flag's own bool
	// (normally true via NoOptDefVal); an explicit --tty=false clears the force.
	if !b {
		f.state.val = !f.positive
		return nil
	}
	f.state.val = f.positive
	return nil
}
