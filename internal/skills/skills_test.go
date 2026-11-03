package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testdata = "../../testdata/skills"

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, _ := os.ReadDir(dir)
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestBuiltinValidAndFirst(t *testing.T) {
	b := Builtin()
	if b.Name != BuiltinName || !b.Builtin || !strings.Contains(b.Body, "get_canvas") {
		t.Fatalf("builtin %+v", b.SkillInfo)
	}
	s := Open(filepath.Join(t.TempDir(), "missing"))
	l := s.List()
	if len(l) != 1 || l[0].Name != BuiltinName || !l[0].Builtin {
		t.Fatalf("empty store list %+v", l)
	}
}

func TestInstallListGetRemove(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	info, err := s.Install(filepath.Join(testdata, "good"), false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "good" || info.Version != "1.2.0" {
		t.Fatalf("info %+v", info)
	}
	l := s.List()
	if len(l) != 2 || l[0].Name != BuiltinName || l[1].Name != "good" {
		t.Fatalf("list %+v", l)
	}
	sk, err := s.Get("good")
	if err != nil || !strings.Contains(sk.Body, "# Good") {
		t.Fatalf("get: %v", err)
	}
	if _, err := s.Install(filepath.Join(testdata, "good"), false); err == nil {
		t.Fatal("reinstall without force accepted")
	}
	if _, err := s.Install(filepath.Join(testdata, "good"), true); err != nil {
		t.Fatalf("force reinstall: %v", err)
	}
	if got := entries(t, dir); len(got) != 1 || got[0] != "good" {
		t.Fatalf("leftovers after force: %v", got)
	}
	if err := s.Remove("good"); err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 1 {
		t.Fatal("remove did not unlist")
	}
	if err := s.Remove(BuiltinName); err == nil {
		t.Fatal("built-in removed")
	}
}

func TestInvalidPackagesLeaveNothing(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	for _, bad := range []string{"Bad_Name", "no-skill-md", "mismatch"} {
		if _, err := s.Install(filepath.Join(testdata, bad), false); err == nil {
			t.Errorf("%s installed", bad)
		}
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("leftovers: %v", got)
	}
}

func writeSkill(t *testing.T, parent, name, desc string) string {
	t.Helper()
	d := filepath.Join(parent, name)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "skill.json"), []byte(`{"name":"`+name+`","version":"1.0.0","description":"`+desc+`"}`), 0o644)
	os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: \""+desc+"\"\n---\n# "+name+"\n"), 0o644)
	return d
}

func TestReservedSymlinkAndSize(t *testing.T) {
	src := t.TempDir()
	s := Open(t.TempDir())
	if _, err := s.Install(writeSkill(t, src, BuiltinName, "Imposter."), false); err == nil {
		t.Fatal("agentcanvas installed")
	}
	d := writeSkill(t, src, "linky", "Has a link.")
	os.Symlink("/etc/hosts", filepath.Join(d, "hosts"))
	if _, err := s.Install(d, false); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink: %v", err)
	}
	big := writeSkill(t, src, "big", "Too large.")
	os.WriteFile(filepath.Join(big, "blob.bin"), make([]byte, MaxPackageBytes+1), 0o644)
	if _, err := s.Install(big, false); err == nil || !strings.Contains(err.Error(), "1 MB") {
		t.Fatalf("size: %v", err)
	}
	if got := entries(t, s.Dir()); len(got) != 0 {
		t.Fatalf("leftovers: %v", got)
	}
}

func TestExport(t *testing.T) {
	s := Open(t.TempDir())
	target := t.TempDir()
	p, err := s.Export(BuiltinName, target, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"SKILL.md", "skill.json"} {
		if _, err := os.Stat(filepath.Join(p, f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	if _, err := Validate(p); err != nil {
		t.Fatalf("exported skill invalid: %v", err)
	}
	if _, err := s.Export(BuiltinName, target, false); err == nil {
		t.Fatal("second export without force accepted")
	}
	if _, err := s.Export(BuiltinName, target, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Export("nope", target, false); err == nil {
		t.Fatal("exporting a missing skill accepted")
	}
}

func TestWatch(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{}, 4)
	go s.Watch(ctx, 20*time.Millisecond, func() { changed <- struct{}{} })
	time.Sleep(50 * time.Millisecond)
	if _, err := s.Install(filepath.Join(testdata, "good"), false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("watch missed the install")
	}
}

func TestFrontmatterQuotes(t *testing.T) {
	fm, err := parseFrontmatter("---\nname: 'x'\ndescription: \"A: b\"\nmeta:\n  nested: 1\n---\nbody")
	if err != nil || fm["name"] != "x" || fm["description"] != "A: b" {
		t.Fatalf("%v %v", fm, err)
	}
}
