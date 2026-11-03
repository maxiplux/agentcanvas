package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentcanvas/internal/mcp"
	"agentcanvas/internal/skills"
)

// splitArgs moves positional arguments after the flags so the standard flag
// package accepts "install PATH --force" as well as "install --force PATH".
func splitArgs(args []string, valueFlags map[string]bool) (flags, pos []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	return flags, pos
}

// ---- attach ----

func healthy(base string) bool {
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Get(base + "/healthz")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func attach(env Env, args []string) int {
	fs := newFlags(env, "attach")
	project := fs.String("project", "", "project directory whose running server to use (default: current directory)")
	rawURL := fs.String("url", "", "MCP URL of a running server (overrides --project)")
	if ok, code := parse(env, fs, args); !ok {
		return code
	}
	endpoint := *rawURL
	label := endpoint
	if endpoint == "" {
		dir, err := absDir(*project)
		if err != nil {
			fmt.Fprintln(env.Stderr, "agent-canvas:", err)
			return ExitErr
		}
		label = dir
		si, err := ReadServerInfo(dir)
		if err != nil || !alive(si, dir) {
			fmt.Fprintf(env.Stderr, "No AgentCanvas server for %s. Start one with: agent-canvas serve\n", dir)
			return ExitErr
		}
		endpoint = si.MCPURL
	} else {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" {
			fmt.Fprintf(env.Stderr, "agent-canvas: invalid --url %q\n", endpoint)
			return ExitErr
		}
		if !healthy(u.Scheme + "://" + u.Host) {
			fmt.Fprintf(env.Stderr, "No AgentCanvas server for %s. Start one with: agent-canvas serve\n", label)
			return ExitErr
		}
	}
	if err := mcp.Attach(env.Ctx, endpoint, env.Stdin, env.Stdout); err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas: attach:", err)
		return ExitErr
	}
	return ExitOK
}

// ---- skills ----

const skillsUsage = `Usage:
  agent-canvas skills list [--json]
  agent-canvas skills install PATH [--force]
  agent-canvas skills remove NAME
  agent-canvas skills export NAME (--dir PATH | --agent claude|codex [--global]) [--force] [--project DIR]
`

// agentPresets are CLI-only path aliases for skills export (§14.4).
var agentPresets = map[string]string{"claude": ".claude", "codex": ".codex"}

func openSkills(env Env) (*skills.Store, bool) {
	dir, err := skills.DefaultDir()
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return nil, false
	}
	return skills.Open(dir), true
}

func skillsCmd(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, skillsUsage)
		return ExitErr
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(env.Stdout, skillsUsage)
		return ExitOK
	}
	sub, rest := args[0], args[1:]
	sk, ok := openSkills(env)
	if !ok {
		return ExitErr
	}
	switch sub {
	case "list":
		fs := newFlags(env, "skills list")
		asJSON := fs.Bool("json", false, "print JSON")
		if ok, code := parse(env, fs, rest); !ok {
			return code
		}
		list := sk.List()
		if *asJSON {
			b, _ := json.MarshalIndent(list, "", "  ")
			fmt.Fprintln(env.Stdout, string(b))
			return ExitOK
		}
		for _, s := range list {
			tag := ""
			if s.Builtin {
				tag = " (built-in)"
			}
			fmt.Fprintf(env.Stdout, "%s %s%s — %s\n", s.Name, s.Version, tag, s.Description)
		}
		return ExitOK
	case "install":
		flagArgs, pos := splitArgs(rest, nil)
		fs := newFlags(env, "skills install")
		force := fs.Bool("force", false, "replace an installed skill of the same name")
		if ok, code := parse(env, fs, flagArgs); !ok {
			return code
		}
		if len(pos) != 1 {
			fmt.Fprint(env.Stderr, skillsUsage)
			return ExitErr
		}
		info, err := sk.Install(pos[0], *force)
		if err != nil {
			fmt.Fprintln(env.Stderr, "agent-canvas:", err)
			return ExitErr
		}
		fmt.Fprintf(env.Stdout, "Installed %s %s into %s\n", info.Name, info.Version, filepath.Join(sk.Dir(), info.Name))
		return ExitOK
	case "remove":
		if len(rest) != 1 || strings.HasPrefix(rest[0], "-") {
			fmt.Fprint(env.Stderr, skillsUsage)
			return ExitErr
		}
		if err := sk.Remove(rest[0]); err != nil {
			fmt.Fprintln(env.Stderr, "agent-canvas:", err)
			return ExitErr
		}
		fmt.Fprintf(env.Stdout, "Removed %s\n", rest[0])
		return ExitOK
	case "export":
		flagArgs, pos := splitArgs(rest, map[string]bool{"dir": true, "agent": true, "project": true})
		fs := newFlags(env, "skills export")
		dir := fs.String("dir", "", "export into this folder")
		agent := fs.String("agent", "", "export into a known agent folder: claude or codex")
		global := fs.Bool("global", false, "with --agent: use the folder in your home directory")
		force := fs.Bool("force", false, "replace an existing export")
		project := fs.String("project", "", "project directory for --agent (default: current directory)")
		if ok, code := parse(env, fs, flagArgs); !ok {
			return code
		}
		if len(pos) != 1 || (*dir == "") == (*agent == "") {
			fmt.Fprint(env.Stderr, skillsUsage)
			return ExitErr
		}
		target := *dir
		if *agent != "" {
			sub, ok := agentPresets[*agent]
			if !ok {
				fmt.Fprintf(env.Stderr, "agent-canvas: unknown --agent %q (use claude or codex, or --dir)\n", *agent)
				return ExitErr
			}
			base := ""
			if *global {
				home, err := os.UserHomeDir()
				if err != nil {
					fmt.Fprintln(env.Stderr, "agent-canvas:", err)
					return ExitErr
				}
				base = home
			} else {
				d, err := absDir(*project)
				if err != nil {
					fmt.Fprintln(env.Stderr, "agent-canvas:", err)
					return ExitErr
				}
				base = d
			}
			target = filepath.Join(base, sub, "skills")
		} else if *global {
			fmt.Fprintln(env.Stderr, "agent-canvas: --global applies only with --agent")
			return ExitErr
		}
		path, err := sk.Export(pos[0], target, *force)
		if err != nil {
			fmt.Fprintln(env.Stderr, "agent-canvas:", err)
			return ExitErr
		}
		fmt.Fprintf(env.Stdout, "Exported %s to %s\n", pos[0], path)
		return ExitOK
	}
	fmt.Fprintf(env.Stderr, "agent-canvas: unknown skills command %q\n\n%s", sub, skillsUsage)
	return ExitErr
}
