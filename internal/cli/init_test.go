package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInitSetsUpEveryAgent(t *testing.T) {
	t.Setenv("AGENT_CANVAS_SKILLS_DIR", t.TempDir())
	if code, _, errs := run(t, "skills", "install", "../../testdata/skills/good"); code != 0 {
		t.Fatalf("install good: %q", errs)
	}
	proj := t.TempDir()
	if err := os.Mkdir(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers":{"other":{"command":"x"}},"keep":1}`)
	writeTestFile(t, filepath.Join(proj, ".codex", "config.toml"),
		"model = \"o3\"\n\n[mcp_servers.agentcanvas]\ncommand = \"old\"\n\n[mcp_servers.agentcanvas.env]\nA = \"1\"\n\n[mcp_servers.other]\ncommand = \"y\"\n")

	code, out, errs := run(t, "init", proj)
	if code != 0 {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if !strings.Contains(out, "created .agentcanvas/project.json") {
		t.Fatalf("project not reported: %q", out)
	}
	if _, err := os.Stat(filepath.Join(proj, ".agentcanvas", "project.json")); err != nil {
		t.Fatal("project.json not created")
	}
	for _, d := range []string{".claude/skills", ".agents/skills"} {
		for _, s := range []string{"agentcanvas", "good"} {
			if _, err := os.Stat(filepath.Join(proj, d, s, "SKILL.md")); err != nil {
				t.Fatalf("missing %s/%s/SKILL.md", d, s)
			}
		}
	}

	var mcpJSON struct {
		MCPServers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
		Keep int `json:"keep"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(proj, ".mcp.json"))), &mcpJSON); err != nil {
		t.Fatal(err)
	}
	ac := mcpJSON.MCPServers["agentcanvas"]
	if mcpJSON.Keep != 1 || mcpJSON.MCPServers["other"].Command != "x" || ac.Type != "stdio" ||
		ac.Command == "" || strings.Join(ac.Args, " ") != "attach --start" {
		t.Fatalf(".mcp.json: %+v", mcpJSON)
	}

	toml := readFile(t, filepath.Join(proj, ".codex", "config.toml"))
	if strings.Count(toml, "[mcp_servers.agentcanvas]") != 1 || strings.Contains(toml, `"old"`) ||
		strings.Contains(toml, "agentcanvas.env") || !strings.Contains(toml, `model = "o3"`) ||
		!strings.Contains(toml, "[mcp_servers.other]") || !strings.Contains(toml, `args = ["attach", "--start"]`) {
		t.Fatalf("config.toml:\n%s", toml)
	}

	gi := readFile(t, filepath.Join(proj, ".gitignore"))
	for _, l := range gitignoreLines {
		if !strings.Contains(gi, l+"\n") {
			t.Fatalf(".gitignore lacks %s:\n%s", l, gi)
		}
	}

	// Re-running refreshes in place: same files, existing project kept.
	before := map[string]string{}
	for _, f := range []string{".mcp.json", ".codex/config.toml", ".gitignore", ".agentcanvas/project.json"} {
		before[f] = readFile(t, filepath.Join(proj, f))
	}
	code, out, _ = run(t, "init", "--project", proj)
	if code != 0 || !strings.Contains(out, "kept the existing .agentcanvas/project.json") {
		t.Fatalf("second init: %d %q", code, out)
	}
	for f, want := range before {
		if got := readFile(t, filepath.Join(proj, f)); got != want {
			t.Fatalf("%s changed on re-run:\n%s\n---\n%s", f, want, got)
		}
	}
}

func TestInitAgentChoiceAndRefusals(t *testing.T) {
	t.Setenv("AGENT_CANVAS_SKILLS_DIR", t.TempDir())
	proj := t.TempDir()
	if code, _, errs := run(t, "init", proj, "--agent", "claude"); code != 0 {
		t.Fatalf("init claude: %q", errs)
	}
	if _, err := os.Stat(filepath.Join(proj, ".agents")); !os.IsNotExist(err) {
		t.Fatal("--agent claude wrote Codex files")
	}
	if _, err := os.Stat(filepath.Join(proj, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal(".gitignore created outside a git repository")
	}
	if code, _, errs := run(t, "init", proj, "--agent", "cursor"); code != 1 || !strings.Contains(errs, "unknown --agent") {
		t.Fatalf("unknown agent: %d %q", code, errs)
	}
	bad := t.TempDir()
	writeTestFile(t, filepath.Join(bad, ".mcp.json"), "{not json")
	if code, _, errs := run(t, "init", bad); code != 1 || !strings.Contains(errs, "not valid JSON") {
		t.Fatalf("invalid .mcp.json: %d %q", code, errs)
	}
	if readFile(t, filepath.Join(bad, ".mcp.json")) != "{not json" {
		t.Fatal("invalid .mcp.json was overwritten")
	}
	if code, _, _ := run(t, "init", proj, "--project", proj); code != 1 {
		t.Fatal("DIR and --project together accepted")
	}
	if code, out, _ := run(t, "init", "--help"); code != 0 || !strings.Contains(out, ".agents/skills") {
		t.Fatal("init --help")
	}
}

// inProcessStarter replaces the detached `serve` with one running in this test
// process, and returns how many times it was asked to start.
func inProcessStarter(t *testing.T) *int {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var exits []chan struct{}
	starts := 0
	prev := startServer
	startServer = func(dir, logPath string) (<-chan struct{}, error) {
		starts++
		ex := make(chan struct{})
		exits = append(exits, ex)
		go func() {
			defer close(ex)
			var b strings.Builder
			Run(Env{Stdin: strings.NewReader(""), Stdout: &b, Stderr: &b, Ctx: ctx}, []string{"serve", "--project", dir})
		}()
		return ex, nil
	}
	t.Cleanup(func() {
		cancel()
		for _, ex := range exits {
			<-ex
		}
		startServer = prev
	})
	return &starts
}

func TestOpenAndAttachStart(t *testing.T) {
	t.Setenv("AGENT_CANVAS_SKILLS_DIR", t.TempDir())
	starts := inProcessStarter(t)
	var opened []string
	prevOpen := openURL
	openURL = func(u string) { opened = append(opened, u) }
	t.Cleanup(func() { openURL = prevOpen })

	proj := t.TempDir()
	code, out, errs := run(t, "open", proj)
	if code != 0 || len(opened) != 1 || !strings.Contains(out, "UI:  "+opened[0]) || !strings.Contains(errs, "studio started") {
		t.Fatalf("open: %d out=%q err=%q opened=%v", code, out, errs, opened)
	}
	if _, err := os.Stat(filepath.Join(proj, ".agentcanvas", "start.lock")); !os.IsNotExist(err) {
		t.Fatal("start.lock left behind")
	}

	// attach --start reuses the running studio and proxies MCP.
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n"
	var o, e strings.Builder
	code = Run(Env{Stdin: strings.NewReader(in), Stdout: &o, Stderr: &e, Ctx: context.Background()}, []string{"attach", "--project", proj, "--start"})
	if code != 0 || !strings.Contains(o.String(), `"get_canvas"`) {
		t.Fatalf("attach --start: %d out=%q err=%q", code, o.String(), e.String())
	}
	if *starts != 1 {
		t.Fatalf("studio started %d times, want 1", *starts)
	}

	// attach --start on a project with no studio starts one.
	other := t.TempDir()
	o.Reset()
	e.Reset()
	code = Run(Env{Stdin: strings.NewReader(in), Stdout: &o, Stderr: &e, Ctx: context.Background()}, []string{"attach", "--project", other, "--start"})
	if code != 0 || *starts != 2 || !strings.Contains(o.String(), `"get_canvas"`) {
		t.Fatalf("attach --start (new): %d starts=%d err=%q", code, *starts, e.String())
	}
	if code, _, _ := run(t, "attach", "--url", "http://127.0.0.1:1/mcp", "--start"); code != 1 {
		t.Fatal("--start with --url accepted")
	}
}

func TestStartReportsAFailedStudio(t *testing.T) {
	prev := startServer
	startServer = func(dir, logPath string) (<-chan struct{}, error) {
		_ = os.WriteFile(logPath, []byte("agent-canvas: no free port in 47321–47341\n"), 0o644)
		ex := make(chan struct{})
		close(ex)
		return ex, nil
	}
	t.Cleanup(func() { startServer = prev })
	code, _, errs := run(t, "open", t.TempDir(), "--no-browser")
	if code != 1 || !strings.Contains(errs, "stopped while starting") || !strings.Contains(errs, "no free port") {
		t.Fatalf("failed start: %d %q", code, errs)
	}
}

func TestServerJSONURLsAreNotTrusted(t *testing.T) {
	t.Setenv("AGENT_CANVAS_SKILLS_DIR", t.TempDir())
	inProcessStarter(t)
	var opened []string
	prevOpen := openURL
	openURL = func(u string) { opened = append(opened, u) }
	t.Cleanup(func() { openURL = prevOpen })
	proj := t.TempDir()
	if code, _, errs := run(t, "open", proj, "--no-browser"); code != 0 {
		t.Fatalf("open: %q", errs)
	}
	si, err := ReadServerInfo(proj)
	if err != nil {
		t.Fatal(err)
	}
	si.UIURL, si.MCPURL = "https://evil.example/", "https://evil.example/mcp"
	b, _ := json.Marshal(si)
	writeTestFile(t, filepath.Join(proj, ".agentcanvas", "server.json"), string(b))
	code, out, _ := run(t, "open", proj)
	want := "http://127.0.0.1:"
	if code != 0 || len(opened) != 1 || !strings.HasPrefix(opened[0], want) || strings.Contains(out, "evil") {
		t.Fatalf("tampered server.json: %d %q opened=%v", code, out, opened)
	}
}

func TestSymlinksRefused(t *testing.T) {
	t.Setenv("AGENT_CANVAS_SKILLS_DIR", t.TempDir())
	elsewhere := filepath.Join(t.TempDir(), "victim")
	writeTestFile(t, elsewhere, "keep me\n")

	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".agentcanvas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(proj, ".agentcanvas", "server.log")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if code, _, errs := run(t, "open", proj, "--no-browser"); code != 1 || !strings.Contains(errs, "symbolic link") {
		t.Fatalf("symlinked server.log: %d %q", code, errs)
	}

	proj2 := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(proj2, ".claude")); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := run(t, "init", proj2, "--agent", "claude"); code != 1 || !strings.Contains(errs, "symbolic link") {
		t.Fatalf("symlinked .claude: %d %q", code, errs)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatal("init wrote through a symlinked .claude")
	}
	if readFile(t, elsewhere) != "keep me\n" {
		t.Fatal("victim file changed")
	}
}
