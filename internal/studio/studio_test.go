package studio

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func newMem(t *testing.T) (*Store, *clock) {
	t.Helper()
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	s, err := Open(t.TempDir(), Options{InMemory: true, Sample: true, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func act(t *testing.T, s *Store, typ string, payload any, opts ...func(*Action)) *Result {
	t.Helper()
	res, err := try(s, typ, payload, opts...)
	if err != nil {
		t.Fatalf("%s: %v", typ, err)
	}
	if errs := CheckInvariants(s.project); len(errs) > 0 {
		t.Fatalf("%s broke invariants: %v", typ, errs)
	}
	return res
}

func try(s *Store, typ string, payload any, opts ...func(*Action)) (*Result, error) {
	b, _ := json.Marshal(payload)
	a := Action{Type: typ, Payload: b}
	for _, o := range opts {
		o(&a)
	}
	return s.Apply(a, "ui")
}

func refused(t *testing.T, s *Store, inv, typ string, payload any) *ActionError {
	t.Helper()
	rev := s.rev
	before, _ := json.Marshal(s.project)
	_, err := try(s, typ, payload)
	var ae *ActionError
	if !errors.As(err, &ae) {
		t.Fatalf("%s: want refusal %s, got %v", typ, inv, err)
	}
	if ae.Invariant != inv {
		t.Fatalf("%s: want invariant %s, got %s (%s)", typ, inv, ae.Invariant, ae.Error())
	}
	after, _ := json.Marshal(s.project)
	if string(before) != string(after) || s.rev != rev {
		t.Fatalf("%s: refused action changed the project or rev", typ)
	}
	return ae
}

func withTab(tab string) func(*Action) { return func(a *Action) { a.Tab = tab } }
func coalesce(k string) func(*Action)  { return func(a *Action) { a.Coalesce = k } }
func commit() func(*Action)            { return func(a *Action) { a.Commit = true } }
func edgeCount(p *Project, from, to string) int {
	n := 0
	for _, e := range p.Edges {
		if e.FromID == from && e.ToID == to {
			n++
		}
	}
	return n
}

func TestSeedExact(t *testing.T) {
	p := MakeSeed()
	if errs := CheckInvariants(p); len(errs) > 0 {
		t.Fatalf("seed invariants: %v", errs)
	}
	if len(p.Flows) != 4 || len(p.Nodes) != 9 || len(p.Edges) != 20 || len(p.Pins) != 2 || p.PinCounter != 2 {
		t.Fatalf("counts: flows %d nodes %d edges %d pins %d", len(p.Flows), len(p.Nodes), len(p.Edges), len(p.Pins))
	}
	want := map[string][4]string{
		"ed_w1": {"pg_home", "pg_shop", "Shop", "primary"}, "ed_w2": {"pg_home", "pg_shop", "Browse the collection", "primary"},
		"ed_w3": {"pg_home", "pg_cart", "Cart", "cross"}, "ed_w4": {"pg_shop", "pg_cart", "Cart", "cross"},
		"ed_w5": {"pg_shop", "pg_cart", "Add the Dawn mug", "cross"}, "ed_w6": {"pg_shop", "pg_cart", "Add to cart", "cross"},
		"ed_w7": {"pg_cart", "pg_shop", "Shop", "cross"}, "ed_w8": {"pg_cart", "pg_confirm", "Pay €28", "primary"},
		"ed_w9": {"pg_cart", "pg_shop", "Keep shopping", "cross"}, "ed_w10": {"pg_confirm", "pg_shop", "Shop", "cross"},
		"ed_w11": {"pg_confirm", "pg_cart", "Cart", "cross"}, "ed_w12": {"pg_confirm", "pg_home", "Back to Hearth", "cross"},
		"ed_m1": {"sc_welcome", "sc_craft", "Get started", "primary"}, "ed_m2": {"sc_welcome", "sc_home", "Skip", "cross"},
		"ed_m3": {"sc_craft", "sc_notify", "Next", "primary"}, "ed_m4": {"sc_notify", "sc_home", "Allow and finish", "cross"},
		"ed_m5": {"sc_notify", "sc_home", "Not now", "cross"}, "ed_m6": {"sc_home", "sc_profile", "Profile", "primary"},
		"ed_m7": {"sc_profile", "sc_home", "Sign out", "back"}, "ed_m8": {"sc_profile", "sc_home", "Back", "back"},
	}
	for id, w := range want {
		e := p.Edge(id)
		if e == nil || e.FromID != w[0] || e.ToID != w[1] || e.Label != w[2] || e.Kind != w[3] {
			t.Errorf("edge %s = %+v, want %v", id, e, w)
		}
	}
	hero := p.Node("pg_home").Block("bk_home_hero")
	if hero.BodyRole != "xs" || hero.Pad != 2 || hero.Gap != 1 || hero.TypeRole != "display" {
		t.Errorf("hero flaw missing: %+v", hero)
	}
	if b := p.Node("pg_shop").Block("bk_shop_cta"); b.ButtonFill != "wash" || b.ButtonInk != "muted" {
		t.Errorf("shop cta flaw missing")
	}
	if n := p.Node("sc_profile"); n.CTA.Size != "sm" {
		t.Errorf("profile cta flaw missing")
	}
	if len(p.Draw.Shapes) != 5 {
		t.Errorf("draw shapes %d", len(p.Draw.Shapes))
	}
	for _, h := range []string{"## Intent", "## Users", "## Principles", "## Decisions"} {
		if !strings.Contains(p.Doc.Markdown, h) {
			t.Errorf("doc missing %s", h)
		}
	}
	if got := len(DocHeadings(p.Doc.Markdown)); got != 5 {
		t.Errorf("doc headings %d, want 5", got)
	}
	// JSON shape: page fields only on pages, null cta stays null
	b, _ := json.Marshal(p.Node("pg_home"))
	if strings.Contains(string(b), "archetype") || !strings.Contains(string(b), `"blocks"`) {
		t.Errorf("page JSON shape: %s", b)
	}
	b, _ = json.Marshal(p.Node("pg_shop").Block("bk_shop_hero"))
	if !strings.Contains(string(b), `"secondary":null`) || strings.Contains(string(b), "links") {
		t.Errorf("hero JSON shape: %s", b)
	}
}

func TestSeedLaneOrder(t *testing.T) {
	p := MakeSeed()
	ids := func(ns []*Node) string {
		var s []string
		for _, n := range ns {
			s = append(s, n.ID)
		}
		return strings.Join(s, ",")
	}
	if got := ids(LaneOrder(p.Flow("fl_onboarding"), p.Nodes, p.Edges)); got != "sc_welcome,sc_craft,sc_notify" {
		t.Errorf("onboarding lane %s", got)
	}
	if got := ids(LaneOrder(p.Flow("fl_marketing"), p.Nodes, p.Edges)); got != "pg_home,pg_shop" {
		t.Errorf("marketing lane %s", got)
	}
}

func TestSeedChecks(t *testing.T) {
	p := MakeSeed()
	f := ComputeChecks(p, "desktop").Failing()
	if len(f.Contrast) != 1 || f.Contrast[0].BlockID != "bk_shop_cta" || f.Contrast[0].Part != "button" || f.Contrast[0].Ratio >= 4.5 {
		t.Fatalf("contrast failures: %+v", f.Contrast)
	}
	if len(f.Tap) != 1 || f.Tap[0].Label != "Profile › Sign out: 36px (min 44)" {
		t.Fatalf("tap failures: %+v", f.Tap)
	}
	// effective role per breakpoint (§9.1)
	if EffectiveRole("display", "phone") != "xl" || EffectiveRole("display", "tablet") != "xl" || EffectiveRole("lg", "tablet") != "lg" || EffectiveRole("lg", "phone") != "md" {
		t.Fatal("effectiveRole")
	}
	if EffectivePad(2, "phone") != 3 || EffectivePad(5, "phone") != 4 || EffectivePad(2, "desktop") != 2 {
		t.Fatal("effectivePad")
	}
}

func TestSameChannelRefused(t *testing.T) {
	s, _ := newMem(t)
	ae := refused(t, s, "same-channel", "set_link_target", map[string]any{
		"nodeId": "pg_home", "owner": map[string]any{"blockId": "bk_home_nav", "childId": "ln_home_story"}, "toId": "sc_home"})
	if !strings.Contains(ae.Error(), "same channel") || !strings.Contains(ae.Error(), "Next:") {
		t.Fatalf("message: %s", ae.Error())
	}
	refused(t, s, "same-channel", "upsert_edge", map[string]any{"fromId": "pg_home", "toId": "sc_home"})
}

func TestSelfEdgeRefusedSelfLinkAllowed(t *testing.T) {
	s, _ := newMem(t)
	refused(t, s, "no-self-edge", "upsert_edge", map[string]any{"fromId": "pg_home", "toId": "pg_home"})
	n := len(s.project.Edges)
	act(t, s, "set_link_target", map[string]any{"nodeId": "pg_home", "owner": map[string]any{"blockId": "bk_home_nav", "childId": "ln_home_story"}, "toId": "pg_home"})
	if len(s.project.Edges) != n {
		t.Fatal("self link created an edge")
	}
}

func TestCrossKind(t *testing.T) {
	s, _ := newMem(t)
	res := act(t, s, "upsert_edge", map[string]any{"fromId": "pg_shop", "toId": "pg_cart", "label": "Buy now"})
	if e := s.project.Edge(res.CreatedID); e.Kind != "cross" {
		t.Fatalf("kind %s", e.Kind)
	}
	refused(t, s, "cross-kind", "upsert_edge", map[string]any{"id": res.CreatedID, "kind": "alt"})
	refused(t, s, "cross-kind", "upsert_edge", map[string]any{"fromId": "pg_home", "toId": "pg_shop", "label": "x", "kind": "cross"})
	act(t, s, "upsert_edge", map[string]any{"id": "ed_w1", "kind": "alt"})
	if s.project.Edge("ed_w1").Kind != "alt" {
		t.Fatal("alt not applied")
	}
	// moving Shop into Checkout makes home→shop cross and shop→cart same-flow
	act(t, s, "upsert_node", map[string]any{"id": "pg_shop", "flowId": "fl_checkout"})
	if s.project.Edge("ed_w1").Kind != "cross" || s.project.Edge("ed_w4").Kind != "primary" {
		t.Fatalf("kinds not recomputed: %s %s", s.project.Edge("ed_w1").Kind, s.project.Edge("ed_w4").Kind)
	}
}

func TestUniqueEdgeTriple(t *testing.T) {
	s, _ := newMem(t)
	refused(t, s, "unique-edge", "upsert_edge", map[string]any{"fromId": "pg_home", "toId": "pg_shop", "label": "Shop"})
	act(t, s, "upsert_edge", map[string]any{"fromId": "pg_home", "toId": "pg_shop", "label": "Error"})
	if edgeCount(s.project, "pg_home", "pg_shop") != 3 {
		t.Fatal("branch edge not added")
	}
}

func TestLinkTargetEdgeSync(t *testing.T) {
	s, _ := newMem(t)
	owner := map[string]any{"blockId": "bk_home_nav", "childId": "ln_home_story"}
	act(t, s, "set_link_target", map[string]any{"nodeId": "pg_home", "owner": owner, "toId": "pg_confirm"})
	r := &reducer{p: s.project}
	if e := r.findEdge("pg_home", "pg_confirm", "Our story"); e == nil || e.Kind != "cross" {
		t.Fatalf("edge not created: %+v", e)
	}
	// relabel keeps the edge
	act(t, s, "update_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_nav", "childId": "ln_home_story", "patch": map[string]any{"label": "Story"}})
	r.p = s.project
	if r.findEdge("pg_home", "pg_confirm", "Story") == nil || r.findEdge("pg_home", "pg_confirm", "Our story") != nil {
		t.Fatal("relabel did not move the edge label")
	}
	// clearing deletes
	act(t, s, "set_link_target", map[string]any{"nodeId": "pg_home", "owner": owner, "toId": nil})
	if edgeCount(s.project, "pg_home", "pg_confirm") != 0 {
		t.Fatal("edge not deleted on clear")
	}
	// toId in update refused
	refused(t, s, "link-target", "update_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_hero", "patch": map[string]any{"cta": map[string]any{"toId": "pg_cart"}}})
	// deleting an edge clears the owning link
	act(t, s, "delete_edge", map[string]any{"edgeId": "ed_w3"})
	for _, l := range s.project.Node("pg_home").Block("bk_home_nav").Links {
		if l.ID == "ln_home_cart" && l.ToID != "" {
			t.Fatal("link toId not cleared by delete_edge")
		}
	}
	// screen CTA and hotspot
	act(t, s, "set_link_target", map[string]any{"nodeId": "sc_home", "owner": map[string]any{"cta": true}, "toId": "sc_profile"})
	act(t, s, "set_link_target", map[string]any{"nodeId": "sc_welcome", "owner": map[string]any{"hotspotId": "hs_skip"}, "toId": nil})
	if edgeCount(s.project, "sc_welcome", "sc_home") != 0 {
		t.Fatal("hotspot edge not removed")
	}
}

func TestDeleteNodeCascade(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "set_current_node", map[string]any{"nodeId": "pg_cart"})
	act(t, s, "add_pin", map[string]any{"kind": "question", "body": "board pin", "target": map[string]any{"type": "board", "nodeId": "pg_cart"}, "x": 0.5, "y": 0.5})
	act(t, s, "delete_node", map[string]any{"nodeId": "pg_cart"})
	p := s.project
	for _, e := range p.Edges {
		if e.FromID == "pg_cart" || e.ToID == "pg_cart" {
			t.Fatalf("edge %s survived", e.ID)
		}
	}
	for _, x := range p.Pins {
		if x.Target.NodeID == "pg_cart" {
			t.Fatal("pin survived")
		}
	}
	if p.Flow("fl_checkout").StartID != "pg_confirm" {
		t.Fatalf("startId %s", p.Flow("fl_checkout").StartID)
	}
	if s.view.CurrentNodeID.Web != "pg_confirm" {
		t.Fatalf("current %s", s.view.CurrentNodeID.Web)
	}
}

func TestFlowCreateDeleteAndLastFlow(t *testing.T) {
	s, _ := newMem(t)
	res := act(t, s, "upsert_flow", map[string]any{"channel": "web"})
	fid := res.CreatedID
	act(t, s, "upsert_flow", map[string]any{"id": fid, "name": "Support"})
	pres := act(t, s, "upsert_node", map[string]any{"flowId": fid, "name": "Help"})
	act(t, s, "insert_block", map[string]any{"nodeId": pres.CreatedID, "type": "nav"})
	act(t, s, "set_link_target", map[string]any{"nodeId": "pg_home", "owner": map[string]any{"blockId": "bk_home_nav", "childId": "ln_home_story"}, "toId": pres.CreatedID})
	act(t, s, "add_pin", map[string]any{"kind": "issue", "body": "x", "target": map[string]any{"type": "node", "nodeId": pres.CreatedID}, "x": 0.1, "y": 0.1})
	act(t, s, "delete_flow", map[string]any{"flowId": fid})
	p := s.project
	if len(p.Nodes) != 9 || len(p.Edges) != 20 || len(p.Pins) != 2 || len(p.Flows) != 4 {
		t.Fatalf("orphans: nodes %d edges %d pins %d flows %d", len(p.Nodes), len(p.Edges), len(p.Pins), len(p.Flows))
	}
	act(t, s, "delete_flow", map[string]any{"flowId": "fl_checkout"})
	refused(t, s, "last-flow", "delete_flow", map[string]any{"flowId": "fl_marketing"})
}

func TestStartIDMustBelong(t *testing.T) {
	s, _ := newMem(t)
	refused(t, s, "start-in-flow", "upsert_flow", map[string]any{"id": "fl_marketing", "startId": "pg_cart"})
	act(t, s, "upsert_flow", map[string]any{"id": "fl_marketing", "startId": "pg_shop"})
}

func TestPinCounterNeverReused(t *testing.T) {
	s, _ := newMem(t)
	refused(t, s, "empty-pin", "add_pin", map[string]any{"kind": "issue", "body": "  ", "target": map[string]any{"type": "draw"}, "x": 1, "y": 1})
	res := act(t, s, "add_pin", map[string]any{"kind": "suggestion", "body": "a", "target": map[string]any{"type": "draw"}, "x": 10, "y": 10})
	if s.project.Pin(res.CreatedID).N != 3 {
		t.Fatal("n != 3")
	}
	act(t, s, "delete_pin", map[string]any{"pinId": res.CreatedID})
	res = act(t, s, "add_pin", map[string]any{"kind": "suggestion", "body": "b", "target": map[string]any{"type": "draw"}, "x": 10, "y": 10})
	if s.project.Pin(res.CreatedID).N != 4 || s.project.PinCounter != 4 {
		t.Fatal("pin number reused")
	}
	act(t, s, "update_pin", map[string]any{"pinId": "pin_1", "resolved": true})
	if OpenIssues(s.project) != 1 {
		t.Fatal("open issues after resolve")
	}
}

func TestSpaceFixed(t *testing.T) {
	s, _ := newMem(t)
	refused(t, s, "fixed-space", "update_tokens", map[string]any{"space": []int{0, 1, 2, 3, 4, 5, 6, 7}})
}

func TestBlockDeleteCascadesPinsAndEdges(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "delete_block", map[string]any{"nodeId": "pg_shop", "blockId": "bk_shop_cta"})
	if s.project.Edge("ed_w6") != nil || s.project.Pin("pin_2") != nil {
		t.Fatal("cta band edge or pin survived")
	}
	// nav delete drops nav-owned edges but keeps hero-owned ones
	act(t, s, "delete_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_nav"})
	if s.project.Edge("ed_w1") != nil || s.project.Edge("ed_w2") == nil {
		t.Fatal("nav cascade wrong")
	}
}

func TestDuplicateBlockSharesEdges(t *testing.T) {
	s, _ := newMem(t)
	res := act(t, s, "duplicate_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_nav"})
	n := s.project.Node("pg_home")
	if n.Blocks[1].ID != res.CreatedID || len(s.project.Edges) != 20 {
		t.Fatalf("dup position or edges: %d", len(s.project.Edges))
	}
	// deleting the copy keeps the original's edges
	act(t, s, "delete_block", map[string]any{"nodeId": "pg_home", "blockId": res.CreatedID})
	if s.project.Edge("ed_w1") == nil {
		t.Fatal("shared edge lost")
	}
}

func TestUndoRedoAndCoalesce(t *testing.T) {
	s, c := newMem(t)
	set := func(h string, opts ...func(*Action)) {
		act(t, s, "update_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_hero", "patch": map[string]any{"headline": h}}, opts...)
	}
	head := func() string { return s.project.Node("pg_home").Block("bk_home_hero").Headline }
	orig := head()
	for _, h := range []string{"P", "Po", "Pot", "Pots"} {
		set(h, coalesce("text:bk_home_hero.headline"))
		c.add(100 * time.Millisecond)
	}
	set("Pots", coalesce("text:bk_home_hero.headline"), commit())
	if n := len(s.hist["web"].past); n != 1 {
		t.Fatalf("coalesced steps = %d, want 1", n)
	}
	act(t, s, "undo", map[string]any{"tab": "web"})
	if head() != orig {
		t.Fatalf("undo gave %q", head())
	}
	act(t, s, "redo", map[string]any{"tab": "web"})
	if head() != "Pots" {
		t.Fatalf("redo gave %q", head())
	}
	// a gap over 600ms starts a new step even with the same key
	set("A", coalesce("k"))
	c.add(700 * time.Millisecond)
	set("B", coalesce("k"))
	if n := len(s.hist["web"].past); n != 3 {
		t.Fatalf("steps = %d, want 3", n)
	}
	refused(t, s, "empty-history", "undo", map[string]any{"tab": "doc"})
}

func TestHistoryCapAndTabs(t *testing.T) {
	s, c := newMem(t)
	for i := 0; i < 51; i++ {
		act(t, s, "update_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_hero", "patch": map[string]any{"headline": strings.Repeat("x", i+1)}})
		c.add(time.Second)
	}
	if n := len(s.hist["web"].past); n != 50 {
		t.Fatalf("cap: %d", n)
	}
	act(t, s, "update_screen", map[string]any{"nodeId": "sc_welcome", "patch": map[string]any{"title": "Hi"}})
	webBefore := s.project.Node("pg_home").Block("bk_home_hero").Headline
	act(t, s, "undo", map[string]any{"tab": "story"})
	if s.project.Node("pg_home").Block("bk_home_hero").Headline != webBefore {
		t.Fatal("story undo touched web")
	}
	if s.project.Node("sc_welcome").Title != "Hearth" {
		t.Fatal("story undo failed")
	}
	for i := 0; i < 50; i++ {
		act(t, s, "undo", map[string]any{"tab": "web"})
	}
	if got := s.project.Node("pg_home").Block("bk_home_hero").Headline; got != "x" {
		t.Fatalf("oldest reachable = %q, want x (51st edit dropped)", got)
	}
}

func TestTokenUndoByTab(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "update_tokens", map[string]any{"colors": map[string]any{"accent": "#0055ff"}}, withTab("web"))
	if s.project.Tokens.Colors["accent"] != "#0055FF" {
		t.Fatal("token not set")
	}
	refused(t, s, "empty-history", "undo", map[string]any{"tab": "story"})
	act(t, s, "undo", map[string]any{"tab": "web"})
	if s.project.Tokens.Colors["accent"] != "#D8432A" {
		t.Fatal("token undo failed")
	}
}

func TestDrawAndDocHistories(t *testing.T) {
	s, _ := newMem(t)
	res := act(t, s, "add_shape", map[string]any{"shape": map[string]any{"type": "rect", "x": 8, "y": 16, "w": 40, "h": 24, "stroke": "ink"}})
	act(t, s, "set_draw_view", map[string]any{"x": 10, "y": 10, "zoom": 2})
	act(t, s, "set_doc", map[string]any{"markdown": "# X"})
	act(t, s, "undo", map[string]any{"tab": "draw"})
	if s.project.Shape(res.CreatedID) != nil {
		t.Fatal("draw undo")
	}
	if s.project.Draw.View.Zoom != 2 || s.project.Doc.Markdown != "# X" {
		t.Fatal("draw undo touched view or doc")
	}
	act(t, s, "undo", map[string]any{"tab": "doc"})
	if s.project.Doc.Markdown != SeedMarkdown {
		t.Fatal("doc undo")
	}
}

func TestUndoFixesView(t *testing.T) {
	s, _ := newMem(t)
	res := act(t, s, "upsert_node", map[string]any{"flowId": "fl_marketing", "name": "About"})
	if s.view.CurrentNodeID.Web != Ref(res.CreatedID) {
		t.Fatal("new node not current")
	}
	act(t, s, "undo", map[string]any{"tab": "web"})
	if s.view.CurrentNodeID.Web != "pg_home" || s.view.Selection != nil {
		t.Fatalf("view not repaired: %+v", s.view)
	}
}

func TestChangedIDs(t *testing.T) {
	s, _ := newMem(t)
	res := act(t, s, "update_block", map[string]any{"nodeId": "pg_home", "blockId": "bk_home_hero", "patch": map[string]any{"headline": "Pots for every table"}})
	got := strings.Join(res.ChangedIDs, ",")
	if got != "pg_home,bk_home_hero" {
		t.Fatalf("changedIds %s", got)
	}
	if !strings.Contains(res.Summary, `Home › Hero headline to "Pots for every table"`) {
		t.Fatalf("summary %s", res.Summary)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{SaveDelay: time.Millisecond, Sample: true})
	if err != nil {
		t.Fatal(err)
	}
	act(t, s, "set_tab", map[string]any{"tab": "story"})
	act(t, s, "set_breakpoint", map[string]any{"breakpoint": "phone"})
	act(t, s, "update_screen", map[string]any{"nodeId": "sc_welcome", "patch": map[string]any{"title": "Hello"}})
	s.Flush()
	s2, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s2.project.Node("sc_welcome").Title != "Hello" || s2.view.Tab != "story" || s2.view.Breakpoint != "phone" {
		t.Fatalf("not restored: %+v", s2.view)
	}
	if s2.view.Selection != nil || s2.view.Preview {
		t.Fatal("selection/preview should reset")
	}
}

func TestCorruptFallsBackToSeed(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, ".agentcanvas")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "project.json"), []byte(`{"key":"agentcanvas.project.v1","project":{"version":2}}`), 0o644)
	os.WriteFile(filepath.Join(data, "ui.json"), []byte(`not json`), 0o644)
	var log strings.Builder
	s, err := Open(dir, Options{Log: &log, Sample: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.project.Edges) != 20 {
		t.Fatal("not seeded")
	}
	m, _ := filepath.Glob(filepath.Join(data, "project.json.bad-*"))
	u, _ := filepath.Glob(filepath.Join(data, "ui.json.bad-*"))
	if len(m) != 1 || len(u) != 1 || !strings.Contains(log.String(), "corrupt") {
		t.Fatalf("quarantine: %v %v %q", m, u, log.String())
	}
}

func TestResetProject(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "delete_node", map[string]any{"nodeId": "pg_home"})
	act(t, s, "reset_project", map[string]any{"template": "sample"})
	if len(s.project.Nodes) != 9 || len(s.hist["web"].past) != 0 || s.project.PinCounter != 2 {
		t.Fatal("reset to sample")
	}
	act(t, s, "reset_project", map[string]any{})
	if len(s.project.Nodes) != 1 || len(s.project.Pins) != 0 || len(s.project.Draw.Shapes) != 0 || s.view.CurrentNodeID.Web != "pg_home" {
		t.Fatalf("reset to blank: %d nodes", len(s.project.Nodes))
	}
	if _, err := s.Apply(Action{Type: "reset_project", Payload: json.RawMessage(`{"template":"demo"}`)}, "ui"); err == nil {
		t.Fatal("unknown template accepted")
	}
}

func TestNewProjectIsBlank(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-shop")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := s.project
	if len(p.Flows) != 2 || len(p.Nodes) != 1 || len(p.Nodes[0].Blocks) != 0 || len(p.Edges) != 0 ||
		len(p.Pins) != 0 || p.PinCounter != 0 || len(p.Draw.Shapes) != 0 {
		t.Fatalf("not blank: %d flows %d nodes %d edges %d pins %d shapes", len(p.Flows), len(p.Nodes), len(p.Edges), len(p.Pins), len(p.Draw.Shapes))
	}
	if !strings.HasPrefix(p.Doc.Markdown, "# my-shop — DESIGN.md") || strings.Contains(p.Doc.Markdown, "Hearth") {
		t.Fatalf("doc: %q", p.Doc.Markdown)
	}
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".agentcanvas", "project.json"))
	if err != nil || strings.Contains(string(b), "Hearth") {
		t.Fatal("blank project not written to disk")
	}
	// Reopening keeps it; a mobile screen can be added to the empty App flow.
	act(t, s, "upsert_node", map[string]any{"flowId": "fl_app"})
	if len(s.project.Nodes) != 2 || s.project.Nodes[1].Kind != "screen" {
		t.Fatal("could not add a screen to the blank App flow")
	}
	if n := OpenIssues(s.project); n != 0 {
		t.Fatalf("blank project reports %d open issues", n)
	}
}

func TestUnknownIDsRefused(t *testing.T) {
	s, _ := newMem(t)
	ae := refused(t, s, "unknown-id", "update_block", map[string]any{"nodeId": "pg_nope", "blockId": "x", "patch": map[string]any{"headline": "y"}})
	if !strings.Contains(ae.Error(), "get_canvas") {
		t.Fatal(ae.Error())
	}
}

func TestSelectionAndView(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "set_selection", map[string]any{"selection": map[string]any{"kind": "block", "nodeId": "pg_home", "blockId": "bk_home_nav", "childId": "ln_home_cart"}})
	act(t, s, "set_tab", map[string]any{"tab": "draw"})
	if s.view.Selection != nil {
		t.Fatal("draw tab should not inherit web selection")
	}
	act(t, s, "set_tab", map[string]any{"tab": "web"})
	if s.view.Selection == nil || s.view.Selection.ChildID != "ln_home_cart" {
		t.Fatal("web selection not restored")
	}
	act(t, s, "set_selection", map[string]any{"selection": map[string]any{"kind": "pin", "pinId": "pin_2"}})
	if s.view.CurrentNodeID.Web != "pg_shop" || s.view.Tab != "web" {
		t.Fatal("pin select did not navigate")
	}
}

// §7.1 lists ed_w11 (Confirm → Cart, both in Checkout) as cross, and §17 row 6 counts
// 9 vermillion seed curves, so the seed keeps it (§0: a table of values beats prose).
// The §6.1 rule still holds for every create/retarget: touching the edge recomputes it,
// and a new same-flow edge is never cross.
func TestEdW11RecomputedOnRetarget(t *testing.T) {
	s, _ := newMem(t)
	if s.project.Edge("ed_w11").Kind != "cross" {
		t.Fatal("seed ed_w11 must match the §7.1 table")
	}
	act(t, s, "upsert_edge", map[string]any{"id": "ed_w11", "toId": "pg_cart", "label": "Cart"})
	if k := s.project.Edge("ed_w11").Kind; k != "primary" {
		t.Fatalf("retargeted same-flow edge kind = %s, want primary", k)
	}
	refused(t, s, "cross-kind", "upsert_edge", map[string]any{"id": "ed_w11", "kind": "cross"})
	res := act(t, s, "upsert_edge", map[string]any{"fromId": "pg_confirm", "toId": "pg_cart", "label": "Edit cart"})
	if k := s.project.Edge(res.CreatedID).Kind; k != "primary" {
		t.Fatalf("new same-flow edge kind = %s", k)
	}
	// a link retarget recomputes too: Confirm's nav Cart link → Shop (cross) → Cart (same flow)
	owner := map[string]any{"blockId": "bk_conf_nav", "childId": "ln_conf_cart"}
	act(t, s, "set_link_target", map[string]any{"nodeId": "pg_confirm", "owner": owner, "toId": "pg_shop"})
	act(t, s, "set_link_target", map[string]any{"nodeId": "pg_confirm", "owner": owner, "toId": "pg_cart"})
	r := &reducer{p: s.project}
	if e := r.findEdge("pg_confirm", "pg_cart", "Cart"); e == nil || e.Kind != "primary" {
		t.Fatalf("relinked edge = %+v, want primary", e)
	}
}

func TestSeedValidates(t *testing.T) {
	if err := Validate(MakeSeed()); err != nil {
		t.Fatalf("seed fails Validate: %v", err)
	}
}

// A hostile project.json (values that would land in exported HTML/CSS) is quarantined.
func TestInjectedProjectQuarantined(t *testing.T) {
	for name, mutate := range map[string]func(p *Project){
		"color":     func(p *Project) { p.Tokens.Colors["accent"] = `red;}</style><script>alert(1)</script>` },
		"blockType": func(p *Project) { p.Nodes[0].Blocks[0].Type = `nav"><script>x</script>` },
		"role":      func(p *Project) { p.Nodes[0].Blocks[1].BodyRole = `md" onmouseover="x` },
		"id":        func(p *Project) { p.Nodes[0].Blocks[0].ID = `bk"><img src=x>` },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			data := filepath.Join(dir, ".agentcanvas")
			os.MkdirAll(data, 0o755)
			p := MakeSeed()
			mutate(p)
			b, _ := json.Marshal(map[string]any{"key": "agentcanvas.project.v1", "project": p})
			os.WriteFile(filepath.Join(data, "project.json"), b, 0o644)
			var log strings.Builder
			s, err := Open(dir, Options{Log: &log, Sample: true})
			if err != nil {
				t.Fatal(err)
			}
			if s.project.Tokens.Colors["accent"] != "#D8432A" || s.project.Nodes[0].Blocks[0].ID != "bk_home_nav" {
				t.Fatal("hostile project was loaded instead of the seed")
			}
			if m, _ := filepath.Glob(filepath.Join(data, "project.json.bad-*")); len(m) != 1 {
				t.Fatalf("not quarantined: %q", log.String())
			}
		})
	}
}

func TestSymlinkedDataDirRefused(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(dir, ".agentcanvas")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := Open(dir, Options{}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(elsewhere, "*")); len(m) != 0 {
		t.Fatalf("wrote through the symlink: %v", m)
	}
}

func TestMoveNodeAndTidy(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "move_node", map[string]any{"nodeId": "pg_shop", "x": 1500.4, "y": -20})
	n := s.project.Node("pg_shop")
	if n.Pos == nil || n.Pos.X != 1500 || n.Pos.Y != -20 {
		t.Fatalf("pos = %+v", n.Pos)
	}
	b, _ := json.Marshal(n)
	if !strings.Contains(string(b), `"pos":{"x":1500,"y":-20}`) {
		t.Fatalf("json %s", b)
	}
	act(t, s, "tidy_flow", map[string]any{"flowId": "fl_marketing"})
	if s.project.Node("pg_shop").Pos != nil {
		t.Fatal("tidy did not clear pos")
	}
	b, _ = json.Marshal(s.project.Node("pg_shop"))
	if strings.Contains(string(b), `"pos"`) {
		t.Fatalf("pos still emitted: %s", b)
	}
	refused(t, s, "invalid-payload", "tidy_flow", map[string]any{"flowId": "fl_marketing"})
	refused(t, s, "unknown-id", "move_node", map[string]any{"nodeId": "nope", "x": 0, "y": 0})
}

func TestMoveNodeHistoryIsChannel(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "move_node", map[string]any{"nodeId": "sc_welcome", "x": 10, "y": 10})
	if !s.historyFlags()["story"].CanUndo || s.historyFlags()["web"].CanUndo {
		t.Fatal("move must go to the story history only")
	}
	act(t, s, "undo", map[string]any{"tab": "story"})
	if s.project.Node("sc_welcome").Pos != nil {
		t.Fatal("undo did not restore pos")
	}
}

func TestCanvasViewDefaults(t *testing.T) {
	s, _ := newMem(t)
	if s.project.Canvas.View.Zoom != 0 {
		t.Fatal("fresh canvas view must be unset (zoom 0)")
	}
	act(t, s, "set_canvas_view", map[string]any{"x": 10, "y": 20, "zoom": 9})
	if v := s.project.Canvas.View; v.X != 10 || v.Y != 20 || v.Zoom != 4 {
		t.Fatalf("view %+v", v)
	}
	if s.historyFlags()["web"].CanUndo || s.historyFlags()["draw"].CanUndo {
		t.Fatal("view must not be undoable")
	}
	// a project.json written before the canvas existed has no "canvas" key
	raw, _ := json.Marshal(MakeSeed())
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(raw, &keys)
	delete(keys, "canvas")
	inner, _ := json.Marshal(keys)
	old := []byte(`{"key":"` + projectKey + `","project":` + string(inner) + `}`)
	p, err := parseProject(old)
	if err != nil || p.Canvas.View.Zoom != 0 {
		t.Fatalf("old project: %v %+v", err, p.Canvas)
	}
}

func TestEnteredClearedOnDelete(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "set_entered", map[string]any{"nodeId": "pg_shop"})
	if s.view.Entered != "pg_shop" || s.view.CurrentNodeID.Web != "pg_shop" {
		t.Fatalf("view %+v", s.view)
	}
	act(t, s, "delete_node", map[string]any{"nodeId": "pg_shop"})
	if s.view.Entered != "" {
		t.Fatal("entered must clear when the node is deleted")
	}
	act(t, s, "set_entered", map[string]any{"nodeId": "sc_welcome"})
	act(t, s, "set_entered", map[string]any{"nodeId": ""})
	if s.view.Entered != "" {
		t.Fatal("empty nodeId must leave")
	}
	if !strings.Contains(string(s.StateJSON("ui", "")), `"name":`) {
		t.Fatal("state must carry the project name")
	}
}

func TestEnteredPersists(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	s, err := Open(dir, Options{Sample: true, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	act(t, s, "set_entered", map[string]any{"nodeId": "pg_cart"})
	act(t, s, "set_canvas_view", map[string]any{"x": 1, "y": 2, "zoom": 0.5})
	s.Flush()
	s2, err := Open(dir, Options{Sample: true, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	if s2.view.Entered != "pg_cart" || s2.project.Canvas.View.Zoom != 0.5 {
		t.Fatalf("after reopen: entered=%q view=%+v", s2.view.Entered, s2.project.Canvas.View)
	}
}
