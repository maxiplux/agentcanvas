package cli

import "fmt"

// SPEC: `open` is an addition to §15 so a coding agent (or a person) can show the
// studio with one command: it starts `serve` for the project in the background
// when none is running (the same path as `attach --start`), opens the UI in the
// default browser, and returns. The studio keeps running after `open` exits.

// openURL opens the UI; tests replace it.
var openURL = openBrowser

const openUsage = `Usage:
  agent-canvas open [DIR] [--no-browser]

Starts the studio for DIR (default: the current directory) in the background if
it is not running, opens the UI in the default browser, and prints the UI and
MCP URLs. --no-browser only starts it and prints the URLs.
`

func openCmd(env Env, args []string) int {
	flagArgs, pos := splitArgs(args, map[string]bool{"project": true})
	fs := newFlags(env, "open")
	project := fs.String("project", "", "project directory (default: current directory)")
	noBrowser := fs.Bool("no-browser", false, "start the studio and print its URLs without opening a browser")
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(env.Stdout, openUsage)
		return ExitOK
	}
	if ok, code := parse(env, fs, flagArgs); !ok {
		return code
	}
	dir, ok := projectDir(env, *project, pos, openUsage)
	if !ok {
		return ExitErr
	}
	si, err := ensureServer(env, dir)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas: open:", err)
		return ExitErr
	}
	if !*noBrowser {
		openURL(si.UIURL)
	}
	fmt.Fprintln(env.Stdout, "UI:  "+si.UIURL)
	fmt.Fprintln(env.Stdout, "MCP: "+si.MCPURL)
	return ExitOK
}
