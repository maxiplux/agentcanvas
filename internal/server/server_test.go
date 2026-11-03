package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentcanvas/internal/hub"
	"agentcanvas/internal/skills"
	"agentcanvas/internal/studio"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := studio.Open(t.TempDir(), studio.Options{InMemory: true, Sample: true})
	if err != nil {
		t.Fatal(err)
	}
	mcp := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	return New(Config{Store: st, Hub: hub.New(), Skills: skills.Open(t.TempDir()), MCP: mcp, Port: 47321}).Handler()
}

func do(h http.Handler, method, path, host, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(`{"type":"set_breakpoint","payload":{"breakpoint":"phone"}}`))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHostGuardBlocksDNSRebinding(t *testing.T) {
	h := testServer(t)
	for _, host := range []string{"evil.example:47321", "attacker.test", "127.0.0.1.evil.example:47321"} {
		for _, path := range []string{"/", "/api/state", "/healthz", "/mcp", "/assets/fonts.css"} {
			if w := do(h, "GET", path, host, ""); w.Code != http.StatusForbidden {
				t.Errorf("Host %s %s: got %d, want 403", host, path, w.Code)
			}
		}
	}
	for _, host := range []string{"127.0.0.1:47321", "localhost:47321", "[::1]:47321"} {
		if w := do(h, "GET", "/api/state", host, ""); w.Code != http.StatusOK {
			t.Errorf("Host %s: got %d, want 200", host, w.Code)
		}
	}
}

func TestAPIOnlyFromOwnOrigin(t *testing.T) {
	h := testServer(t)
	cases := map[string]int{
		"http://127.0.0.1:47321": 200, "http://localhost:47321": 200,
		"http://127.0.0.1:3000": 403, "http://localhost:5173": 403, "http://evil.example": 403, "https://127.0.0.1:47321": 403,
	}
	for origin, want := range cases {
		if w := do(h, "POST", "/api/act", "127.0.0.1:47321", origin); w.Code != want {
			t.Errorf("POST /api/act Origin %s: got %d, want %d", origin, w.Code, want)
		}
	}
}

func TestMCPAllowsLoopbackOriginsWithCORS(t *testing.T) {
	h := testServer(t)
	w := do(h, "POST", "/mcp", "127.0.0.1:47321", "http://localhost:6274")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:6274" {
		t.Fatalf("loopback MCP client: %d %q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w := do(h, "POST", "/mcp", "127.0.0.1:47321", "http://evil.example"); w.Code != 403 {
		t.Fatalf("foreign origin on /mcp: %d", w.Code)
	}
	if w := do(h, "OPTIONS", "/mcp", "127.0.0.1:47321", "http://127.0.0.1:9999"); w.Code != 204 {
		t.Fatalf("preflight: %d", w.Code)
	}
}
