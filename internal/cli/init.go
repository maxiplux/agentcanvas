package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"agentcanvas/internal/skills"
	"agentcanvas/internal/studio"
)

// SPEC: `init` is an addition to §15, requested by the project owner. It sets a
// project up for coding agents in one command: the .agentcanvas project, every
// skill exported where each agent looks for skills, an MCP server entry that
// runs `agent-canvas attach --start`, and .gitignore lines for per-machine
// files. Like the `skills export` presets, agent names live only in the CLI.
// Codex skills go to .agents/skills (where current Codex loads repository
// skills); the §14.4 `--agent codex` export preset keeps its .codex/skills path.

const initUsage = `Usage:
  agent-canvas init [DIR] [--agent claude,codex] [--sample]

Sets DIR (default: the current directory) up for AgentCanvas and coding agents:
  .agentcanvas/project.json   the design project: blank if missing (--sample: the Hearth
                              sample); an existing project is never overwritten
  .claude/skills/<skill>/     every skill, for Claude Code         (--agent claude)
  .mcp.json                   MCP server "agentcanvas" for Claude Code
  .agents/skills/<skill>/     every skill, for Codex               (--agent codex)
  .codex/config.toml          [mcp_servers.agentcanvas] for Codex
  .gitignore                  per-machine .agentcanvas files (in git repositories)
The MCP entry runs "agent-canvas attach --start", which starts the studio when it
is not running. Run init again at any time: it refreshes skills and config in place.
`

// mcpServerName is the key init writes in every agent's MCP config.
const mcpServerName = "agentcanvas"

var mcpArgs = []string{"attach", "--start"}

type agentTarget struct {
	name      string
	skillsDir string // relative to the project
	config    string // relative to the project
	write     func(path, command string) error
}

var initAgents = []agentTarget{
	{name: "claude", skillsDir: filepath.Join(".claude", "skills"), config: ".mcp.json", write: writeJSONMCP},
	{name: "codex", skillsDir: filepath.Join(".agents", "skills"), config: filepath.Join(".codex", "config.toml"), write: writeTOMLMCP},
}

// gitignoreLines are per-machine files that should not be committed.
var gitignoreLines = []string{
	".agentcanvas/server.json",
	".agentcanvas/ui.json",
	".agentcanvas/server.log",
	".agentcanvas/start.lock",
}

func initCmd(env Env, args []string) int {
	flagArgs, pos := splitArgs(args, map[string]bool{"agent": true, "project": true})
	fs := newFlags(env, "init")
	agentList := fs.String("agent", "claude,codex", "agents to set up, comma-separated: claude, codex")
	project := fs.String("project", "", "project directory (default: current directory)")
	sample := fs.Bool("sample", false, "create the project from the Hearth sample instead of a blank canvas")
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(env.Stdout, initUsage)
		return ExitOK
	}
	if ok, code := parse(env, fs, flagArgs); !ok {
		return code
	}
	dir, ok := projectDir(env, *project, pos, initUsage)
	if !ok {
		return ExitErr
	}
	targets, err := pickAgents(*agentList)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return ExitErr
	}
	sk, ok := openSkills(env)
	if !ok {
		return ExitErr
	}
	out := env.Stdout
	fmt.Fprintf(out, "Setting up AgentCanvas in %s\n", dir)

	msg, err := initProject(env, dir, *sample)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return ExitErr
	}
	fmt.Fprintf(out, "  project  %s\n", msg)

	command, note := mcpCommand()
	names := skillNames(sk)
	for _, a := range targets {
		if err := a.setup(sk, dir, names, command); err != nil {
			fmt.Fprintf(env.Stderr, "agent-canvas: %s: %v\n", a.name, err)
			return ExitErr
		}
		fmt.Fprintf(out, "  %-7s  skills %s → %s\n", a.name, strings.Join(names, ", "), filepath.ToSlash(a.skillsDir)+"/")
		fmt.Fprintf(out, "  %-7s  MCP    %s → %s %s\n", "", filepath.ToSlash(a.config), command, strings.Join(mcpArgs, " "))
	}

	added, err := ensureGitignore(dir)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas: .gitignore:", err)
		return ExitErr
	}
	if len(added) > 0 {
		fmt.Fprintf(out, "  git      .gitignore += %s\n", strings.Join(added, " "))
	}
	if note != "" {
		fmt.Fprintf(out, "\nNote: %s\n", note)
	}
	fmt.Fprint(out, `
Next:
  Restart your coding agent (or reload its MCP servers) so it loads the skill and the
  "agentcanvas" MCP server, then ask it to design with AgentCanvas. The agent starts
  the studio when it needs it; open the UI yourself (or let the agent) with: agent-canvas open
`)
	if targets[len(targets)-1].name == "codex" {
		fmt.Fprintln(out, "  Codex reads .codex/config.toml only for projects you have marked as trusted.")
	}
	fmt.Fprintln(out, "  Run agent-canvas init again after installing skills or updating to refresh them.")
	return ExitOK
}

// setup exports every skill into the agent's skills folder and writes its MCP entry.
func (a agentTarget) setup(sk *skills.Store, dir string, names []string, command string) error {
	for _, rel := range []string{filepath.Dir(a.skillsDir), a.skillsDir, filepath.Dir(a.config), a.config} {
		if rel == "." {
			continue
		}
		if err := refuseSymlink(filepath.Join(dir, rel)); err != nil {
			return err
		}
	}
	for _, n := range names {
		if _, err := sk.Export(n, filepath.Join(dir, a.skillsDir), true); err != nil {
			return fmt.Errorf("exporting skill %s: %w", n, err)
		}
	}
	return a.write(filepath.Join(dir, a.config), command)
}

// projectDir resolves the project from an optional positional DIR or --project.
func projectDir(env Env, project string, pos []string, usage string) (string, bool) {
	if len(pos) > 1 || (len(pos) == 1 && project != "") {
		fmt.Fprint(env.Stderr, usage)
		return "", false
	}
	if len(pos) == 1 {
		project = pos[0]
	}
	dir, err := absDir(project)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return "", false
	}
	return dir, true
}

func pickAgents(list string) ([]agentTarget, error) {
	want := map[string]bool{}
	for _, n := range strings.Split(list, ",") {
		n = strings.ToLower(strings.TrimSpace(n))
		switch n {
		case "":
		case "all":
			for _, a := range initAgents {
				want[a.name] = true
			}
		case "claude", "codex":
			want[n] = true
		default:
			return nil, fmt.Errorf("unknown --agent %q (use claude, codex, or both: claude,codex)", n)
		}
	}
	var out []agentTarget
	for _, a := range initAgents {
		if want[a.name] {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--agent needs at least one of: claude, codex")
	}
	return out, nil
}

// initProject seeds .agentcanvas/project.json unless the project already exists.
func initProject(env Env, dir string, sample bool) (string, error) {
	if si, ok := runningServer(dir); ok {
		return "studio already running at " + si.UIURL, nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".agentcanvas", "project.json")); err == nil {
		return "kept the existing .agentcanvas/project.json", nil
	}
	st, err := studio.Open(dir, studio.Options{Log: env.Stderr, Sample: sample})
	if err != nil {
		return "", err
	}
	st.Flush()
	if sample {
		return "created .agentcanvas/project.json from the Hearth sample", nil
	}
	return "created .agentcanvas/project.json (a blank canvas)", nil
}

func skillNames(sk *skills.Store) []string {
	var names []string
	for _, s := range sk.List() {
		names = append(names, s.Name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		// the built-in skill first, then the rest by name
		if (names[i] == skills.BuiltinName) != (names[j] == skills.BuiltinName) {
			return names[i] == skills.BuiltinName
		}
		return names[i] < names[j]
	})
	return names
}

// mcpCommand is the command MCP configs run: the bare name when agent-canvas is
// on PATH (portable across machines), otherwise this binary's full path.
func mcpCommand() (string, string) {
	if _, err := exec.LookPath("agent-canvas"); err == nil {
		return "agent-canvas", ""
	}
	exe, err := selfPath()
	if err != nil {
		return "agent-canvas", ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, "agent-canvas is not on your PATH, so the MCP config uses its full path (" + exe + ")."
}

// writeJSONMCP merges the agentcanvas server into a JSON "mcpServers" config,
// keeping every other server and key.
func writeJSONMCP(path, command string) error {
	doc := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case err == nil && len(bytes.TrimSpace(b)) > 0:
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s is not valid JSON (%v); fix it, then run init again", path, err)
		}
	case err != nil && !os.IsNotExist(err):
		return err
	}
	servers := map[string]any{}
	if v, ok := doc["mcpServers"]; ok {
		m, isObj := v.(map[string]any)
		if !isObj {
			return fmt.Errorf("%s: \"mcpServers\" is not an object; fix it, then run init again", path)
		}
		servers = m
	}
	servers[mcpServerName] = map[string]any{"type": "stdio", "command": command, "args": mcpArgs}
	doc["mcpServers"] = servers
	nb, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(nb, '\n'))
}

// writeTOMLMCP writes the [mcp_servers.agentcanvas] table into a TOML config,
// replacing an earlier copy (and its sub-tables) and leaving the rest untouched.
func writeTOMLMCP(path, command string) error {
	quoted := make([]string, len(mcpArgs))
	for i, a := range mcpArgs {
		quoted[i] = tomlString(a)
	}
	header := "[mcp_servers." + mcpServerName + "]"
	block := []string{header, "command = " + tomlString(command), "args = [" + strings.Join(quoted, ", ") + "]"}

	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var kept []string
	inOurs, replaced := false, false
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			inOurs = t == header || strings.HasPrefix(t, "[mcp_servers."+mcpServerName+".")
			if inOurs && !replaced {
				kept = append(kept, block...)
				replaced = true
			}
		}
		if !inOurs {
			kept = append(kept, line)
		}
	}
	text := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if !replaced {
		if text != "" {
			text += "\n\n"
		}
		text += strings.Join(block, "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(text+"\n"))
}

func tomlString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// ensureGitignore adds the per-machine .agentcanvas files to .gitignore when
// the project is a git repository (or already has a .gitignore).
func ensureGitignore(dir string) ([]string, error) {
	path := filepath.Join(dir, ".gitignore")
	if err := refuseSymlink(path); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if _, gerr := os.Stat(filepath.Join(dir, ".git")); gerr != nil {
			return nil, nil
		}
	} else if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var added []string
	for _, l := range gitignoreLines {
		if !have[l] {
			added = append(added, l)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	text := string(b)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "# AgentCanvas: per-machine files\n" + strings.Join(added, "\n") + "\n"
	return added, writeFileAtomic(path, []byte(text))
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
