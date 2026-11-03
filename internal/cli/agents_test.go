package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errw strings.Builder
	code := Run(Env{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errw, Ctx: context.Background()}, args)
	return code, out.String(), errw.String()
}

func TestSkillsCLI(t *testing.T) {
	t.Setenv("AGENT_CANVAS_SKILLS_DIR", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	proj := t.TempDir()

	if code, out, _ := run(t, "skills", "list"); code != 0 || !strings.Contains(out, "agentcanvas 1.0.0 (built-in)") {
		t.Fatalf("list: %d %q", code, out)
	}
	if code, _, errs := run(t, "skills", "remove", "agentcanvas"); code != 1 || !strings.Contains(errs, "built-in") {
		t.Fatalf("remove built-in: %d %q", code, errs)
	}
	if code, _, errs := run(t, "skills", "install", "../../testdata/skills/mismatch"); code != 1 || errs == "" {
		t.Fatalf("bad install: %d", code)
	}
	if code, out, errs := run(t, "skills", "install", "../../testdata/skills/good"); code != 0 {
		t.Fatalf("install: %d %q %q", code, out, errs)
	}
	if code, out, _ := run(t, "skills", "list", "--json"); code != 0 || !strings.Contains(out, `"name": "good"`) {
		t.Fatalf("list json: %q", out)
	}
	if code, _, errs := run(t, "skills", "install", "../../testdata/skills/good"); code != 1 || !strings.Contains(errs, "force") {
		t.Fatalf("reinstall: %d %q", code, errs)
	}
	if code, _, _ := run(t, "skills", "install", "../../testdata/skills/good", "--force"); code != 0 {
		t.Fatal("force reinstall")
	}

	// export presets (§14.4, §17 row 43)
	if code, _, errs := run(t, "skills", "export", "agentcanvas", "--agent", "claude", "--project", proj); code != 0 {
		t.Fatalf("export claude: %q", errs)
	}
	for _, f := range []string{"SKILL.md", "skill.json"} {
		if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "agentcanvas", f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	if code, _, _ := run(t, "skills", "export", "agentcanvas", "--agent", "claude", "--project", proj); code != 1 {
		t.Fatal("second export without --force accepted")
	}
	if code, _, _ := run(t, "skills", "export", "agentcanvas", "--agent", "claude", "--project", proj, "--force"); code != 0 {
		t.Fatal("export --force")
	}
	if code, _, errs := run(t, "skills", "export", "agentcanvas", "--agent", "claude", "--global"); code != 0 {
		t.Fatalf("global: %q", errs)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "agentcanvas", "SKILL.md")); err != nil {
		t.Fatal("global export missing")
	}
	if code, _, _ := run(t, "skills", "export", "good", "--agent", "codex", "--project", proj); code != 0 {
		t.Fatal("export codex")
	}
	if _, err := os.Stat(filepath.Join(proj, ".codex", "skills", "good", "SKILL.md")); err != nil {
		t.Fatal("codex export missing")
	}
	x := filepath.Join(t.TempDir(), "X")
	if code, _, _ := run(t, "skills", "export", "agentcanvas", "--dir", x); code != 0 {
		t.Fatal("export --dir")
	}
	if _, err := os.Stat(filepath.Join(x, "agentcanvas", "skill.json")); err != nil {
		t.Fatal("--dir export missing")
	}
	if code, _, _ := run(t, "skills", "remove", "good"); code != 0 {
		t.Fatal("remove good")
	}
	if code, out, _ := run(t, "skills", "list"); code != 0 || strings.Contains(out, "good") {
		t.Fatalf("good still listed: %q", out)
	}
}

func TestAttachNoServer(t *testing.T) {
	dir := t.TempDir()
	code, _, errs := run(t, "attach", "--project", dir)
	want := "No AgentCanvas server for " + dir + ". Start one with: agent-canvas serve"
	if code != 1 || strings.TrimSpace(errs) != want {
		t.Fatalf("attach: %d %q", code, errs)
	}
	code, _, errs = run(t, "attach", "--url", "http://127.0.0.1:1/mcp")
	if code != 1 || !strings.Contains(errs, "Start one with: agent-canvas serve") {
		t.Fatalf("attach --url: %d %q", code, errs)
	}
}

func TestVersionAndUnknown(t *testing.T) {
	if code, out, _ := run(t, "version"); code != 0 || !strings.HasPrefix(out, "agent-canvas ") {
		t.Fatal("version")
	}
	if code, _, _ := run(t, "bogus"); code != 1 {
		t.Fatal("unknown command")
	}
}
