package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SPEC: `agent-canvas update` is an addition to §15, requested by the project owner
// with the one-line installers: it replaces the running binary with a GitHub release,
// verified against that release's SHA256SUMS. It only touches the network when run.

const defaultReleasesURL = "https://github.com/maxiplux/agentcanvas/releases"

// selfPath locates the binary to replace; tests point it at a temp file.
var selfPath = os.Executable

func releasesURL() string {
	if v := os.Getenv("AGENT_CANVAS_RELEASES_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultReleasesURL
}

func assetName(goos, goarch string) string {
	n := "agent-canvas-" + goos + "-" + goarch
	if goos == "windows" {
		n += ".exe"
	}
	return n
}

// latestTag reads the tag from the /releases/latest redirect, so no API token or
// rate-limited endpoint is involved.
func latestTag(client *http.Client, base string) (string, error) {
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := c.Get(base + "/latest")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if res.StatusCode/100 != 3 || i < 0 {
		return "", fmt.Errorf("could not find the latest release at %s/latest (HTTP %d)", base, res.StatusCode)
	}
	return url.PathUnescape(loc[i+len("/tag/"):])
}

func download(client *http.Client, u string, w io.Writer) error {
	res, err := client.Get(u)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", u, res.StatusCode)
	}
	_, err = io.Copy(w, res.Body)
	return err
}

func expectedSum(sums, asset string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s is not listed in SHA256SUMS", asset)
}

func update(env Env, args []string) int {
	fs := newFlags(env, "update")
	check := fs.Bool("check", false, "only report whether a newer release exists")
	want := fs.String("version", "", "install this release tag instead of the latest")
	if ok, code := parse(env, fs, args); !ok {
		return code
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	base := releasesURL()
	tag := *want
	if tag == "" {
		t, err := latestTag(client, base)
		if err != nil {
			fmt.Fprintln(env.Stderr, "agent-canvas: update:", err)
			return ExitErr
		}
		tag = t
	}
	if tag == Version {
		fmt.Fprintf(env.Stdout, "agent-canvas %s is up to date\n", Version)
		return ExitOK
	}
	if *check {
		fmt.Fprintf(env.Stdout, "agent-canvas %s is installed; %s is available. Run: agent-canvas update\n", Version, tag)
		return ExitOK
	}
	exe, err := selfPath()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas: update: cannot locate the running binary:", err)
		return ExitErr
	}
	if err := replaceBinary(client, base, tag, assetName(runtime.GOOS, runtime.GOARCH), exe, runtime.GOOS == "windows"); err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas: update:", err)
		return ExitErr
	}
	fmt.Fprintf(env.Stdout, "Updated agent-canvas %s → %s at %s\n", Version, tag, exe)
	return ExitOK
}

// replaceBinary downloads asset from release tag, verifies it, and swaps it in for exe.
func replaceBinary(client *http.Client, base, tag, asset, exe string, windows bool) error {
	dl := base + "/download/" + url.PathEscape(tag)
	var sums strings.Builder
	if err := download(client, dl+"/SHA256SUMS", &sums); err != nil {
		return err
	}
	wantSum, err := expectedSum(sums.String(), asset)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".agent-canvas-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (%v); re-run the installer, or move agent-canvas to a folder you own", dir, err)
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	if err := download(client, dl+"/"+asset, io.MultiWriter(tmp, h)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSum {
		return fmt.Errorf("checksum mismatch for %s (want %s, got %s); nothing was changed", asset, wantSum, got)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if windows {
		// A running .exe cannot be overwritten, but it can be renamed out of the way.
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(tmp.Name(), exe)
}
