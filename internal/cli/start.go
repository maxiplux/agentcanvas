package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SPEC: `attach --start` is an addition to §15 that goes with `init`. Plain
// `attach` keeps the §15 behaviour (exit 1 when no server runs); with --start it
// first launches `serve` for the project in the background, so an MCP config
// written by `init` works whether or not the designer has the studio open.

const startTimeout = 15 * time.Second

// startServer launches a detached `serve` for dir and returns a channel that is
// closed if that process exits. Tests replace it with an in-process server.
var startServer = func(dir, logPath string) (<-chan struct{}, error) {
	exe, err := selfPath()
	if err != nil {
		return nil, err
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND|openNoFollow, 0o644)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "serve", "--project", dir)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logf, logf
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	return exited, nil
}

// ensureServer returns the running server for dir, starting one if needed.
func ensureServer(env Env, dir string) (*ServerInfo, error) {
	if si, ok := runningServer(dir); ok {
		return si, nil
	}
	data := filepath.Join(dir, ".agentcanvas")
	for _, p := range []string{data, filepath.Join(data, "server.log"), filepath.Join(data, "start.lock")} {
		if err := refuseSymlink(p); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		return nil, err
	}
	// One starter at a time: two agents attaching at once must not start two servers.
	lock := filepath.Join(data, "start.lock")
	if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > 2*startTimeout {
		_ = os.Remove(lock) // left behind by a starter that died
	}
	var exited <-chan struct{}
	logPath := filepath.Join(data, "server.log")
	if f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err == nil {
		f.Close()
		defer os.Remove(lock)
		fmt.Fprintf(env.Stderr, "agent-canvas: no studio running for %s; starting one\n", dir)
		if exited, err = startServer(dir, logPath); err != nil {
			return nil, fmt.Errorf("could not start the studio: %w", err)
		}
	}
	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		if si, ok := runningServer(dir); ok {
			if exited != nil {
				fmt.Fprintf(env.Stderr, "agent-canvas: studio started — UI: %s (log: %s)\n", si.UIURL, logPath)
			}
			return si, nil
		}
		select {
		case <-exited:
			return nil, fmt.Errorf("the studio stopped while starting: %s", lastLines(logPath, 3))
		case <-env.Ctx.Done():
			return nil, env.Ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil, errors.New("timed out waiting for the studio to start; see " + logPath)
}

// refuseSymlink fails when path exists and is a symbolic link, so a cloned
// repository cannot redirect a write to a file elsewhere on the machine.
func refuseSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link; refusing to write through it", path)
	}
	return nil
}

func lastLines(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(no log)"
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
