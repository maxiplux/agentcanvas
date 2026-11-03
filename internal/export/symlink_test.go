package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentcanvas/internal/studio"
)

func TestWriteRefusesSymlinkedExportDir(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agentcanvas"), 0o755)
	if err := os.Symlink(elsewhere, filepath.Join(dir, ".agentcanvas", "exports")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	f, err := Generate(studio.MakeSeed(), "draw-svg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, "draw-svg", f); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(elsewhere, "*")); len(m) != 0 {
		t.Fatalf("wrote through the symlink: %v", m)
	}
}

func TestColorSanitized(t *testing.T) {
	p := studio.MakeSeed()
	p.Tokens.Colors["accent"] = "red;}</style><script>alert(1)</script>"
	f, err := Generate(p, "page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(f.Body), "<script>alert") {
		t.Fatal("unsanitized color reached the export")
	}
}
