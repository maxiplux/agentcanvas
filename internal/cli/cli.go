// Package cli implements the agent-canvas command line (§15).
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"agentcanvas/internal/hub"
	"agentcanvas/internal/mcp"
	"agentcanvas/internal/server"
	"agentcanvas/internal/skills"
	"agentcanvas/internal/studio"
)

// Version is set at build time with -ldflags "-X agentcanvas/internal/cli.Version=…".
var Version = "0.1.0-dev"

// Exit codes (§15).
const (
	ExitOK      = 0
	ExitErr     = 1
	ExitNoPort  = 2
	ExitRunning = 3
)

const usage = `agent-canvas — a local design studio that people and MCP agents edit together.

Usage:
  agent-canvas [serve] [--project DIR] [--port N] [--open] [--stdio] [--dev]
  agent-canvas init [DIR] [--agent claude,codex]
  agent-canvas open [DIR] [--no-browser]
  agent-canvas attach [--project DIR [--start] | --url URL]
  agent-canvas skills list [--json]
  agent-canvas skills install PATH [--force]
  agent-canvas skills remove NAME
  agent-canvas skills export NAME (--dir PATH | --agent claude|codex [--global]) [--force]
  agent-canvas update [--check] [--version TAG]
  agent-canvas version
  agent-canvas --help

serve    Start the studio for the project in DIR (default: current directory).
         The project lives in DIR/.agentcanvas/. Prints the UI URL and the MCP URL.
         --stdio also speaks MCP on stdin/stdout (logs go to stderr).
         --dev serves React development builds and reads web/ from disk.
init     Set a project up for coding agents: the .agentcanvas project, every skill in
         .claude/skills and .agents/skills, and an "agentcanvas" MCP server in
         .mcp.json and .codex/config.toml. Safe to re-run; it refreshes in place.
open     Start the studio in the background if needed and open the UI in a browser.
attach   Proxy MCP between stdin/stdout and the server already running for DIR.
         --start starts the studio in the background first when none is running.
skills   Manage SKILL.md packages (stored in the user config dir).
update   Replace this binary with the latest GitHub release (verified by SHA-256).
         --check only reports whether a newer release exists.
`

// Env carries process streams so tests can run the CLI in-process.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Ctx    context.Context
}

// Main runs the CLI with process streams and returns the exit code.
func Main(args []string) int {
	return Run(Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Ctx: context.Background()}, args)
}

// Run executes args (without the program name).
func Run(env Env, args []string) int {
	if env.Ctx == nil {
		env.Ctx = context.Background()
	}
	cmd := "serve"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve(env, args)
	case "init":
		return initCmd(env, args)
	case "open":
		return openCmd(env, args)
	case "attach":
		return attach(env, args)
	case "skills":
		return skillsCmd(env, args)
	case "update":
		return update(env, args)
	case "version":
		fmt.Fprintln(env.Stdout, "agent-canvas", Version)
		return ExitOK
	case "help":
		fmt.Fprint(env.Stdout, usage)
		return ExitOK
	}
	fmt.Fprintf(env.Stderr, "agent-canvas: unknown command %q\n\n%s", cmd, usage)
	return ExitErr
}

func newFlags(env Env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		// Intentionally empty: parse prints usage once (stdout for --help, stderr for mistakes).
	}
	return fs
}

func parse(env Env, fs *flag.FlagSet, args []string) (ok bool, code int) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(env.Stdout, usage)
			return false, ExitOK
		}
		fmt.Fprint(env.Stderr, "\n"+usage)
		return false, ExitErr
	}
	return true, ExitOK
}

// ServerInfo is .agentcanvas/server.json (§12.1).
type ServerInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	UIURL     string `json:"uiUrl"`
	MCPURL    string `json:"mcpUrl"`
	StartedAt int64  `json:"startedAt"`
}

func serverJSONPath(projectDir string) string {
	return filepath.Join(projectDir, ".agentcanvas", "server.json")
}

// ReadServerInfo reads server.json for a project directory.
func ReadServerInfo(projectDir string) (*ServerInfo, error) {
	b, err := os.ReadFile(serverJSONPath(projectDir))
	if err != nil {
		return nil, err
	}
	var si ServerInfo
	if err := json.Unmarshal(b, &si); err != nil {
		return nil, err
	}
	return &si, nil
}

// alive reports whether a server answers /healthz for this project.
func alive(si *ServerInfo, projectDir string) bool {
	c := http.Client{Timeout: time.Second}
	res, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", si.Port))
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var h struct {
		OK      bool   `json:"ok"`
		Project string `json:"project"`
	}
	if json.NewDecoder(res.Body).Decode(&h) != nil || !h.OK {
		return false
	}
	return h.Project == "" || h.Project == projectDir
}

func absDir(d string) (string, error) {
	if d == "" {
		d = "."
	}
	abs, err := filepath.Abs(d)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}

func findWebDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "web", "index.html")); err == nil {
			return filepath.Join(d, "web"), nil
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return "", errors.New("--dev must run inside the agentcanvas repository (web/index.html not found)")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func serve(env Env, args []string) int {
	fs := newFlags(env, "serve")
	project := fs.String("project", "", "project directory (default: current directory)")
	port := fs.Int("port", 0, "port to bind (default: first free in 47321–47341)")
	open := fs.Bool("open", false, "open the UI in the default browser")
	stdio := fs.Bool("stdio", false, "also serve MCP on stdin/stdout")
	dev := fs.Bool("dev", false, "serve development React builds and read web/ from disk")
	if ok, code := parse(env, fs, args); !ok {
		return code
	}
	logw := env.Stdout
	if *stdio {
		logw = env.Stderr
	}
	dir, err := absDir(*project)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return ExitErr
	}
	if si, err := ReadServerInfo(dir); err == nil && alive(si, dir) {
		fmt.Fprintf(env.Stderr, "agent-canvas: a server is already running for %s\n", dir)
		fmt.Fprintln(logw, "UI:  "+si.UIURL)
		fmt.Fprintln(logw, "MCP: "+si.MCPURL)
		return ExitRunning
	}
	webDir := ""
	if *dev {
		if webDir, err = findWebDir(); err != nil {
			fmt.Fprintln(env.Stderr, "agent-canvas:", err)
			return ExitErr
		}
	}
	ln, bound, err := server.Listen(*port)
	if err != nil {
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return ExitNoPort
	}
	store, err := studio.Open(dir, studio.Options{Log: env.Stderr})
	if err != nil {
		ln.Close()
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return ExitErr
	}
	skDir, err := skills.DefaultDir()
	if err != nil {
		ln.Close()
		fmt.Fprintln(env.Stderr, "agent-canvas:", err)
		return ExitErr
	}
	sk := skills.Open(skDir)
	store.SetSkills(sk.List)
	h := hub.New()
	store.SetPublisher(h)
	m := mcp.New(mcp.Options{Store: store, Skills: sk, Version: Version})
	srv := server.New(server.Config{Store: store, Hub: h, Skills: sk, MCP: m.Handler(), Guide: m.Guide,
		Version: Version, Port: bound, Dev: *dev, WebDir: webDir, Log: env.Stderr})
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(env.Ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	si := ServerInfo{PID: os.Getpid(), Port: bound, UIURL: server.UIURL(bound), MCPURL: server.MCPURL(bound), StartedAt: time.Now().Unix()}
	if b, err := json.MarshalIndent(si, "", "  "); err == nil {
		_ = os.WriteFile(serverJSONPath(dir), append(b, '\n'), 0o644)
	}
	defer os.Remove(serverJSONPath(dir))

	go sk.Watch(ctx, 2*time.Second, func() { store.Notify("cli", "Skills changed") })

	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()

	fmt.Fprintln(logw, "UI:  "+si.UIURL)
	fmt.Fprintln(logw, "MCP: "+si.MCPURL)
	fmt.Fprintln(logw, `Any streamable-HTTP MCP client can connect to the MCP URL; stdio clients can run "agent-canvas attach".`)
	if *open {
		openBrowser(si.UIURL)
	}

	stdioDone := make(chan struct{})
	if *stdio {
		go func() {
			defer close(stdioDone)
			if err := m.RunStdio(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintln(env.Stderr, "agent-canvas: stdio:", err)
			}
		}()
	}

	code := ExitOK
	select {
	case <-ctx.Done():
	case <-stdioDone:
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(env.Stderr, "agent-canvas:", err)
			code = ExitErr
		}
	}
	h.Close() // end SSE streams so Shutdown does not wait on them
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(sctx)
	store.Flush()
	return code
}
