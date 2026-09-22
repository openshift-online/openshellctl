package cli

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestExec_NoCommandExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "exec", "--name", "demo")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "a command is required") {
		t.Errorf("msg = %v", err)
	}
}

func TestExec_BadEnvExit2(t *testing.T) {
	_, err := runCmd(t, "sandbox", "exec", "--name", "demo", "--env", "1BAD=x", "--", "echo")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
}

func TestTTYTriState_LastWins(t *testing.T) {
	// Neither flag → nil (auto-detect).
	st := &ttyTriState{}
	if st.Value() != nil {
		t.Error("unset should be nil")
	}

	// --tty then --no-tty → last wins (false).
	st = &ttyTriState{}
	tf := ttyStateFlag{st, true}
	nf := ttyStateFlag{st, false}
	_ = tf.Set("true")
	_ = nf.Set("true")
	if v := st.Value(); v == nil || *v != false {
		t.Errorf("--tty --no-tty → %v, want false", v)
	}

	// --no-tty then --tty → last wins (true).
	st = &ttyTriState{}
	tf = ttyStateFlag{st, true}
	nf = ttyStateFlag{st, false}
	_ = nf.Set("true")
	_ = tf.Set("true")
	if v := st.Value(); v == nil || *v != true {
		t.Errorf("--no-tty --tty → %v, want true", v)
	}
}

func TestLogs_BadSinceExit2(t *testing.T) {
	_, err := runCmd(t, "logs", "demo", "--since", "5x")
	if err == nil || exitCodeFor(err) != ExitUsage {
		t.Fatalf("err = %v exit = %d, want usage", err, exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "unknown duration unit: x") {
		t.Errorf("msg = %v", err)
	}
}

func TestLogs_HasLgAlias(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := NewRootCommand()
	names := collectCommandNames(root)
	lg, ok := names["logs"]
	if !ok {
		t.Fatal("logs command missing")
	}
	found := false
	for _, a := range lg.Aliases {
		if a == "lg" {
			found = true
		}
	}
	if !found {
		t.Errorf("logs missing 'lg' alias, got %v", lg.Aliases)
	}
}

func TestExec_HasShortNameFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := NewRootCommand()
	names := collectCommandNames(root)
	exec := names["exec"]
	if exec == nil {
		t.Fatal("exec command missing")
	}
	if f := exec.Flags().ShorthandLookup("n"); f == nil || f.Name != "name" {
		t.Errorf("exec -n should map to --name, got %v", f)
	}
}

func TestRemoteExitErrorMapping(t *testing.T) {
	if got := exitCodeFor(&RemoteExitError{Code: 42}); got != 42 {
		t.Errorf("RemoteExitError exit = %d, want 42", got)
	}
	if exitCodeError(0) != nil {
		t.Error("exitCodeError(0) should be nil")
	}
	if err := exitCodeError(7); err == nil || exitCodeFor(err) != 7 {
		t.Errorf("exitCodeError(7) exit = %d", exitCodeFor(err))
	}
}
