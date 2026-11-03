package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeReleases serves /releases/latest (redirect), SHA256SUMS, and one asset.
func fakeReleases(t *testing.T, tag string, body []byte, sum string) *httptest.Server {
	t.Helper()
	asset := assetName(runtime.GOOS, runtime.GOARCH)
	if sum == "" {
		h := sha256.Sum256(body)
		sum = hex.EncodeToString(h[:])
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/"+tag+"/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0000  agent-canvas-other\n" + sum + "  " + asset + "\n"))
	})
	mux.HandleFunc("/releases/download/"+tag+"/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("AGENT_CANVAS_RELEASES_URL", srv.URL+"/releases")
	return srv
}

func withSelf(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent-canvas")
	os.WriteFile(p, []byte(content), 0o755)
	old := selfPath
	selfPath = func() (string, error) { return p, nil }
	t.Cleanup(func() { selfPath = old })
	return p
}

func withVersion(t *testing.T, v string) {
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(Env{Stdout: &out, Stderr: &errb}, args)
	return code, out.String(), errb.String()
}

func TestUpdateReplacesBinary(t *testing.T) {
	fakeReleases(t, "v9.9.9", []byte("NEW BINARY"), "")
	self := withSelf(t, "OLD BINARY")
	withVersion(t, "v0.1.0")
	code, out, errs := runCLI("update")
	got, _ := os.ReadFile(self)
	if code != 0 || string(got) != "NEW BINARY" || !strings.Contains(out, "v0.1.0 → v9.9.9") {
		t.Fatalf("code %d out %q err %q content %q", code, out, errs, got)
	}
	if fi, _ := os.Stat(self); fi.Mode().Perm()&0o100 == 0 {
		t.Fatal("updated binary is not executable")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(self), ".agent-canvas-update-*")); len(m) != 0 {
		t.Fatalf("temp file left behind: %v", m)
	}
}

func TestUpdateRefusesBadChecksum(t *testing.T) {
	fakeReleases(t, "v9.9.9", []byte("TAMPERED"), strings.Repeat("ab", 32))
	self := withSelf(t, "OLD BINARY")
	withVersion(t, "v0.1.0")
	code, _, errs := runCLI("update")
	got, _ := os.ReadFile(self)
	if code != ExitErr || string(got) != "OLD BINARY" || !strings.Contains(errs, "checksum mismatch") {
		t.Fatalf("code %d err %q content %q", code, errs, got)
	}
}

func TestUpdateCheckAndUpToDate(t *testing.T) {
	fakeReleases(t, "v9.9.9", []byte("NEW"), "")
	self := withSelf(t, "OLD")
	withVersion(t, "v0.1.0")
	if code, out, _ := runCLI("update", "--check"); code != 0 || !strings.Contains(out, "v9.9.9 is available") {
		t.Fatalf("--check: %d %q", code, out)
	}
	withVersion(t, "v9.9.9")
	if code, out, _ := runCLI("update"); code != 0 || !strings.Contains(out, "up to date") {
		t.Fatalf("up to date: %d %q", code, out)
	}
	if got, _ := os.ReadFile(self); string(got) != "OLD" {
		t.Fatal("binary changed although nothing should have")
	}
}

func TestUpdatePinnedVersion(t *testing.T) {
	fakeReleases(t, "v1.2.3", []byte("PINNED"), "")
	self := withSelf(t, "OLD")
	withVersion(t, "v0.1.0")
	if code, _, errs := runCLI("update", "--version", "v1.2.3"); code != 0 {
		t.Fatalf("pinned update failed: %s", errs)
	}
	if got, _ := os.ReadFile(self); string(got) != "PINNED" {
		t.Fatalf("content %q", got)
	}
}
