package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"agentcanvas/internal/skills"
	"agentcanvas/internal/studio"
)

// catalog is every §13.3 tool.
var catalog = []string{
	"get_canvas", "get_node", "get_doc", "skills_list",
	"set_tab", "set_breakpoint", "set_selection", "set_toggles", "preview",
	"undo", "redo", "reset_project",
	"update_tokens", "upsert_flow", "delete_flow", "upsert_node", "reorder_nodes", "delete_node",
	"insert_block", "update_block", "upsert_child", "delete_child", "reorder_blocks", "duplicate_block", "delete_block",
	"update_screen", "upsert_hotspot", "delete_hotspot", "set_link_target", "upsert_edge", "delete_edge",
	"add_shape", "update_shape", "duplicate_shape", "delete_shape", "set_doc", "add_pin", "update_pin", "delete_pin",
	"export", "skills_install", "skills_remove",
}

type fixture struct {
	srv   *Server
	store *studio.Store
	cs    *sdk.ClientSession
	ctx   context.Context
}

func setup(t *testing.T) *fixture {
	t.Helper()
	st, err := studio.Open(t.TempDir(), studio.Options{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	sk := skills.Open(t.TempDir())
	st.SetSkills(sk.List)
	s := New(Options{Store: st, Skills: sk, Version: "test"})
	ctx := context.Background()
	ct, stTr := sdk.NewInMemoryTransports()
	ss, err := s.SDK().Connect(ctx, stTr, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &fixture{srv: s, store: st, cs: cs, ctx: ctx}
}

func (f *fixture) call(t *testing.T, name string, args any) (string, bool) {
	t.Helper()
	res, err := f.cs.CallTool(f.ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n"), res.IsError
}

func (f *fixture) canvas(t *testing.T) map[string]any {
	t.Helper()
	txt, isErr := f.call(t, "get_canvas", map[string]any{})
	if isErr {
		t.Fatal(txt)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(txt), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestToolsListComplete(t *testing.T) {
	f := setup(t)
	res, err := f.cs.ListTools(f.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, n := range catalog {
		if !got[n] {
			t.Errorf("missing tool %s", n)
		}
	}
	if len(res.Tools) != len(catalog) {
		t.Errorf("tools: got %d, want %d", len(res.Tools), len(catalog))
	}
}

func TestGetCanvasSeed(t *testing.T) {
	f := setup(t)
	c := f.canvas(t)
	counts := c["counts"].(map[string]any)
	if counts["flows"].(float64) != 4 || counts["nodes"].(float64) != 9 || counts["edges"].(float64) != 20 {
		t.Fatalf("counts %v", counts)
	}
	pins := c["pins"].([]any)
	if len(pins) != 2 || pins[0].(map[string]any)["n"].(float64) != 1 || pins[1].(map[string]any)["n"].(float64) != 2 {
		t.Fatalf("pins %v", pins)
	}
	checks := c["checks"].(map[string]any)
	contrast := checks["contrast"].([]any)
	if len(contrast) != 1 || contrast[0].(map[string]any)["blockId"] != "bk_shop_cta" {
		t.Fatalf("contrast %v", contrast)
	}
	tap := checks["tap"].([]any)
	if len(tap) != 1 || tap[0].(map[string]any)["label"] != "Profile › Sign out: 36px (min 44)" {
		t.Fatalf("tap %v", tap)
	}
	sk := c["skills"].([]any)
	if len(sk) == 0 || sk[0] != "agentcanvas" {
		t.Fatalf("skills %v", sk)
	}
}

func TestSameChannelRefusalKeepsRev(t *testing.T) {
	f := setup(t)
	rev := f.canvas(t)["rev"]
	txt, isErr := f.call(t, "set_link_target", map[string]any{
		"nodeId": "pg_home", "owner": map[string]any{"blockId": "bk_home_nav", "childId": "ln_home_story"}, "toId": "sc_home"})
	if !isErr || !strings.Contains(txt, "same channel") || !strings.Contains(txt, "Next:") {
		t.Fatalf("want same-channel refusal, got %v %q", isErr, txt)
	}
	if f.canvas(t)["rev"] != rev {
		t.Fatal("rev changed on refusal")
	}
	// unknown ids name the id and suggest get_canvas
	txt, isErr = f.call(t, "get_node", map[string]any{"nodeId": "pg_nope"})
	if !isErr || !strings.Contains(txt, "pg_nope") || !strings.Contains(txt, "get_canvas") {
		t.Fatalf("unknown id: %q", txt)
	}
}

func TestUpdateBlockResult(t *testing.T) {
	f := setup(t)
	txt, isErr := f.call(t, "update_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_hero", "patch": map[string]any{"headline": "Pots for every table"}})
	if isErr {
		t.Fatal(txt)
	}
	var r studio.Result
	if err := json.Unmarshal([]byte(txt), &r); err != nil {
		t.Fatal(err)
	}
	if !r.OK || !strings.Contains(r.Summary, `Home › Hero headline to "Pots for every table"`) || len(r.ChangedIDs) == 0 {
		t.Fatalf("result %+v", r)
	}
	node, _ := f.call(t, "get_node", map[string]any{"nodeId": "pg_home"})
	if !strings.Contains(node, "Pots for every table") {
		t.Fatal("get_node does not show the edit")
	}
	// undo defaults to the shared current tab (web)
	if txt, isErr := f.call(t, "undo", map[string]any{}); isErr {
		t.Fatal(txt)
	}
	node, _ = f.call(t, "get_node", map[string]any{"nodeId": "pg_home"})
	if strings.Contains(node, "Pots for every table") {
		t.Fatal("undo did not revert")
	}
}

func TestResetNeedsConfirm(t *testing.T) {
	f := setup(t)
	if txt, isErr := f.call(t, "reset_project", map[string]any{"confirm": false}); !isErr || !strings.Contains(txt, "confirm") {
		t.Fatalf("reset without confirm: %q", txt)
	}
	if txt, isErr := f.call(t, "reset_project", map[string]any{"confirm": true}); isErr {
		t.Fatal(txt)
	}
}

func TestTokensSpaceRefused(t *testing.T) {
	f := setup(t)
	txt, isErr := f.call(t, "update_tokens", map[string]any{"space": []int{0, 2, 4, 8, 16, 24, 32, 48}})
	if !isErr || !strings.Contains(txt, "fixed") {
		t.Fatalf("space: %q", txt)
	}
	txt, isErr = f.call(t, "update_tokens", map[string]any{"colors": map[string]any{"accent": "#0055FF"}})
	if isErr {
		t.Fatal(txt)
	}
}

func TestSkillsTools(t *testing.T) {
	f := setup(t)
	txt, _ := f.call(t, "skills_list", map[string]any{})
	var list []skills.Skill
	if err := json.Unmarshal([]byte(txt), &list); err != nil || len(list) == 0 || !list[0].Builtin || list[0].Body == "" {
		t.Fatalf("skills_list: %v %q", err, txt)
	}
	if txt, isErr := f.call(t, "skills_remove", map[string]any{"name": "agentcanvas"}); !isErr {
		t.Fatalf("removing built-in: %q", txt)
	}
}

func TestGuideAndSkillMentionEveryTool(t *testing.T) {
	f := setup(t)
	guide := f.srv.Guide()
	body := skills.Builtin().Body
	if !strings.Contains(guide, "get_canvas") || !strings.Contains(guide, "first") {
		t.Fatal("guide must say to call get_canvas first")
	}
	for _, tl := range f.srv.Tools() {
		tick := "`" + tl.Name + "`"
		if !strings.Contains(guide, tick) {
			t.Errorf("guide misses %s", tl.Name)
		}
		if !strings.Contains(body, tick) {
			t.Errorf("SKILL.md misses %s", tl.Name)
		}
	}
	// SKILL.md must not name tools that are not registered
	registered := map[string]bool{}
	for _, tl := range f.srv.Tools() {
		registered[tl.Name] = true
	}
	for _, n := range catalog {
		if !registered[n] {
			t.Errorf("catalog tool %s not registered", n)
		}
	}
	res, err := f.cs.ReadResource(f.ctx, &sdk.ReadResourceParams{URI: GuideURI})
	if err != nil || len(res.Contents) != 1 || res.Contents[0].Text != guide {
		t.Fatalf("guide resource mismatch: %v", err)
	}
}

func TestNoVendorNames(t *testing.T) {
	f := setup(t)
	vendors := []string{"claude", "codex", "cursor", "openai", "anthropic", "copilot", "gemini"}
	texts := []string{Instructions, f.srv.Guide()}
	for _, tl := range f.srv.Tools() {
		texts = append(texts, tl.Name, tl.Description)
	}
	res, _ := f.cs.ListTools(f.ctx, nil)
	for _, tl := range res.Tools {
		b, _ := json.Marshal(tl.InputSchema)
		texts = append(texts, string(b))
	}
	for _, s := range texts {
		low := strings.ToLower(s)
		for _, v := range vendors {
			if strings.Contains(low, v) {
				t.Errorf("vendor name %q in %q", v, s[:min(len(s), 80)])
			}
		}
	}
}

func TestSchemaEnums(t *testing.T) {
	f := setup(t)
	res, _ := f.cs.ListTools(f.ctx, nil)
	for _, tl := range res.Tools {
		if tl.Name == "set_tab" {
			b, _ := json.Marshal(tl.InputSchema)
			if !strings.Contains(string(b), `"enum":["web","story","draw","doc"]`) {
				t.Fatalf("set_tab schema: %s", b)
			}
		}
	}
	// schema validation refuses an unknown tab before the reducer
	if txt, isErr := f.call(t, "set_tab", map[string]any{"tab": "nope"}); !isErr {
		t.Fatalf("bad tab accepted: %q", txt)
	}
}

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// TestRawHTTP mirrors the §17 row 37 curl sequence: only Content-Type and Accept headers.
func TestRawHTTP(t *testing.T) {
	f := setup(t)
	hs := httptest.NewServer(f.srv.Handler())
	defer hs.Close()
	code, body := post(t, hs.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}`)
	if code != 200 || !strings.Contains(body, `Call `+"`get_canvas`"+` before editing`) {
		t.Fatalf("initialize %d %s", code, body)
	}
	code, body = post(t, hs.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if code != 200 || !strings.Contains(body, `"get_canvas"`) {
		t.Fatalf("tools/list %d %s", code, body[:min(len(body), 200)])
	}
	code, body = post(t, hs.URL, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_canvas","arguments":{}}}`)
	if code != 200 || !strings.Contains(body, `pg_home`) {
		t.Fatalf("get_canvas %d %s", code, body[:min(len(body), 200)])
	}
}

// TestAttachProxy drives the stdio proxy with a real go-sdk client.
func TestAttachProxy(t *testing.T) {
	f := setup(t)
	hs := httptest.NewServer(f.srv.Handler())
	defer hs.Close()
	clientToProxyR, clientToProxyW := io.Pipe()
	proxyToClientR, proxyToClientW := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Attach(ctx, hs.URL, clientToProxyR, proxyToClientW)
		proxyToClientW.Close()
	}()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "stdio-client", Version: "1"}, nil).
		Connect(ctx, &sdk.IOTransport{Reader: proxyToClientR, Writer: clientToProxyW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != len(catalog) {
		t.Fatalf("list via attach: %v", err)
	}
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "update_block", Arguments: map[string]any{
		"nodeId": "pg_home", "blockId": "bk_home_hero", "patch": map[string]any{"headline": "Via attach"}}})
	if err != nil || res.IsError {
		t.Fatalf("call via attach: %v %+v", err, res)
	}
	if !strings.Contains(string(f.store.StateJSON("", "")), "Via attach") {
		t.Fatal("attach edit not in the store")
	}
	cs.Close()
	clientToProxyW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("attach did not return on EOF")
	}
}

// TestAttachRawLines checks notifications produce no output and requests one line each.
func TestAttachRawLines(t *testing.T) {
	f := setup(t)
	hs := httptest.NewServer(f.srv.Handler())
	defer hs.Close()
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"raw","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_doc","arguments":{}}}`,
	}, "\n") + "\n")
	var out strings.Builder
	if err := Attach(context.Background(), hs.URL, in, &out); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	sc.Buffer(nil, 1<<20)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":1`) || !strings.Contains(lines[1], "Hearth") {
		t.Fatalf("lines: %v", lines)
	}
}
