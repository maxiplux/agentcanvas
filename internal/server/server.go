// Package server is the AgentCanvas HTTP surface: the studio page, vendored
// assets, the browser API with SSE, exports, skills, discovery, and /mcp (§4, §12.2, §13.2).
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentcanvas/internal/export"
	"agentcanvas/internal/hub"
	"agentcanvas/internal/skills"
	"agentcanvas/internal/studio"
	"agentcanvas/web"
)

// FirstPort and PortSpan define the scan range 47321–47341 (§4.5).
const (
	FirstPort = 47321
	PortSpan  = 21
)

// ErrNoPort means every port in the range is taken.
var ErrNoPort = errors.New("no free port in 47321–47341")

// Listen binds 127.0.0.1, scanning the range unless port is non-zero.
func Listen(port int) (net.Listener, int, error) {
	if port != 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrNoPort, err)
		}
		return ln, port, nil
	}
	for p := FirstPort; p < FirstPort+PortSpan; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, p, nil
		}
	}
	return nil, 0, ErrNoPort
}

// Config wires the server.
type Config struct {
	Store   *studio.Store
	Hub     *hub.Hub
	Skills  *skills.Store
	MCP     http.Handler
	Guide   func() string
	Version string
	Port    int
	Dev     bool   // serve React development builds and read web/ from disk
	WebDir  string // disk path of web/ in dev mode
	Log     io.Writer
}

// Server is the HTTP handler set.
type Server struct {
	c      Config
	webFS  fs.FS
	assets fs.FS
}

// New builds a Server.
func New(c Config) *Server {
	s := &Server{c: c}
	if c.Log == nil {
		s.c.Log = os.Stderr
	}
	if c.Dev && c.WebDir != "" {
		s.webFS = os.DirFS(c.WebDir)
	} else {
		s.webFS = web.FS
	}
	sub, err := fs.Sub(s.webFS, "vendor")
	if err != nil {
		panic(err)
	}
	s.assets = sub
	return s
}

// UIURL and MCPURL for a bound port.
func UIURL(port int) string  { return fmt.Sprintf("http://127.0.0.1:%d/", port) }
func MCPURL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d/mcp", port) }

// Handler returns the full mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", s.assetHandler()))
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /guide", s.guide)
	mux.HandleFunc("GET /mcp.json", s.mcpJSON)

	api := s.sameOrigin
	mux.Handle("GET /api/state", api(http.HandlerFunc(s.state)))
	mux.Handle("POST /api/act", api(http.HandlerFunc(s.act)))
	mux.Handle("GET /api/events", api(http.HandlerFunc(s.events)))
	mux.Handle("GET /api/export/{kind}", api(http.HandlerFunc(s.exportFile)))
	// SPEC: the Export menu's "Save DESIGN.md to project" needs a write endpoint; §12.2 lists none.
	mux.Handle("POST /api/export/{kind}/write", api(http.HandlerFunc(s.exportWrite)))
	mux.Handle("GET /api/skills", api(http.HandlerFunc(s.skillList)))
	mux.Handle("GET /api/skills/{name}", api(http.HandlerFunc(s.skillGet)))

	if s.c.MCP != nil {
		// SPEC: §4.5 accepts any loopback Origin on /mcp and sends CORS to it, so browser-based
		// MCP clients served from other local ports can connect; /api is limited to our own origin.
		mux.Handle("/mcp", loopbackOnly(cors(s.c.MCP)))
	}
	return hostGuard(mux)
}

// ---- guards ----

func loopbackHost(h string) bool { return h == "127.0.0.1" || h == "localhost" || h == "::1" }

func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	return loopbackHost(u.Hostname())
}

// hostGuard rejects every request whose Host is not a loopback name. A DNS-rebinding
// page reaches 127.0.0.1 with its own hostname in Host, and same-origin GETs carry
// no Origin header, so the Host check is what keeps it from reading the project (§4.5).
func hostGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if hn, _, err := net.SplitHostPort(host); err == nil {
			host = hn
		}
		if !loopbackHost(strings.Trim(host, "[]")) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// loopbackOnly rejects requests carrying a non-loopback Origin (§4.5).
func loopbackOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !loopbackOrigin(o) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// sameOrigin allows /api requests only from the studio page this server serves:
// no Origin header, or http://{127.0.0.1|localhost|[::1]}:<bound port>.
func (s *Server) sameOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || u.Scheme != "http" || !loopbackHost(u.Hostname()) || u.Port() != strconv.Itoa(s.c.Port) {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// cors adds CORS headers for loopback origins on /mcp only.
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && loopbackOrigin(o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-ID")
			w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// ---- page and assets ----

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(s.webFS, "index.html")
	if err != nil {
		http.Error(w, "index.html missing: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if s.c.Dev {
		b = []byte(strings.NewReplacer(
			"react.production.min.js", "react.development.js",
			"react-dom.production.min.js", "react-dom.development.js",
		).Replace(string(b)))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

func (s *Server) assetHandler() http.Handler {
	fsrv := http.FileServer(http.FS(s.assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") || r.URL.Path == "VERSIONS" {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			w.Header().Set("Content-Type", "font/woff2")
		}
		if !s.c.Dev {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		fsrv.ServeHTTP(w, r)
	})
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.c.Version, "project": s.c.Store.Dir()})
}

func (s *Server) guide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	g := ""
	if s.c.Guide != nil {
		g = s.c.Guide()
	}
	io.WriteString(w, g)
}

func (s *Server) mcpJSON(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"mcpServers": map[string]any{"agentcanvas": map[string]any{"type": "http", "url": MCPURL(s.c.Port)}}})
}

// ---- browser API ----

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(s.c.Store.StateJSON("", ""))
}

func (s *Server) act(w http.ResponseWriter, r *http.Request) {
	var a studio.Action
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	if err := dec.Decode(&a); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad action JSON: " + err.Error()})
		return
	}
	res, err := s.c.Store.Apply(a, "ui")
	if err != nil {
		var ae *studio.ActionError
		if errors.As(err, &ae) {
			writeJSON(w, ae.Status, map[string]any{"ok": false, "error": ae.Error(), "invariant": ae.Invariant, "next": ae.Next})
			return
		}
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	sub := s.c.Hub.Subscribe()
	defer s.c.Hub.Unsubscribe(sub)
	write := func(data []byte) error {
		if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", data); err != nil {
			return err
		}
		fl.Flush()
		return nil
	}
	if err := write(s.c.Store.StateJSON("ui", "")); err != nil {
		return
	}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-sub.C:
			if !ok {
				return // dropped (slow) or shutting down; client reconnects
			}
			if err := write(data); err != nil {
				return
			}
		case <-tick.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) exportFile(w http.ResponseWriter, r *http.Request) {
	p, _, _ := s.c.Store.Snapshot()
	f, err := export.Generate(p, r.PathValue("kind"), r.URL.Query())
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Name))
	w.Write(f.Body)
}

func (s *Server) exportWrite(w http.ResponseWriter, r *http.Request) {
	p, _, _ := s.c.Store.Snapshot()
	kind := r.PathValue("kind")
	f, err := export.Generate(p, kind, r.URL.Query())
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	path, err := export.Write(s.c.Store.Dir(), kind, f)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	rel, _ := filepath.Rel(s.c.Store.Dir(), path)
	writeJSON(w, 200, map[string]any{"ok": true, "path": path, "rel": rel})
}

func (s *Server) skillList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.c.Skills.List())
}

func (s *Server) skillGet(w http.ResponseWriter, r *http.Request) {
	sk, err := s.c.Skills.Get(r.PathValue("name"))
	if err != nil {
		writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, sk)
}
