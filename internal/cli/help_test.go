package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpPrintsUsageOnceToStdout(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"serve", "--help"}, {"help"}} {
		var out, errb bytes.Buffer
		code := Run(Env{Stdout: &out, Stderr: &errb}, args)
		if code != ExitOK || strings.Count(out.String(), "Usage:") != 1 || errb.Len() != 0 {
			t.Errorf("%v: code %d, stdout usage x%d, stderr %q", args, code, strings.Count(out.String(), "Usage:"), errb.String())
		}
	}
}

func TestBadFlagPrintsErrorAndUsageToStderr(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run(Env{Stdout: &out, Stderr: &errb}, []string{"serve", "--nope"})
	if code != ExitErr || out.Len() != 0 || !strings.Contains(errb.String(), "flag provided but not defined: -nope") || strings.Count(errb.String(), "Usage:") != 1 {
		t.Fatalf("code %d stdout %q stderr %q", code, out.String(), errb.String())
	}
}
