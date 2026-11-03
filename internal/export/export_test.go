package export

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"agentcanvas/internal/studio"
)

func gen(t *testing.T, p *studio.Project, kind string, q url.Values) *File {
	t.Helper()
	f, err := Generate(p, kind, q)
	if err != nil {
		t.Fatalf("%s: %v", kind, err)
	}
	return f
}

func noPins(t *testing.T, p *studio.Project, kind, body string) {
	t.Helper()
	if strings.Contains(body, "pin_") || strings.Contains(strings.ToLower(body), "class=\"pin") {
		t.Errorf("%s contains pin markup", kind)
	}
	for _, x := range p.Pins {
		if strings.Contains(body, x.Body) || strings.Contains(body, esc(x.Body)) {
			t.Errorf("%s contains pin %d body", kind, x.N)
		}
	}
}

func TestFileNames(t *testing.T) {
	p := studio.MakeSeed()
	cases := []struct {
		kind string
		q    url.Values
		name string
		ct   string
	}{
		{"page", url.Values{"nodeId": {"pg_home"}}, "hearth-home.html", ctHTML},
		{"page", nil, "hearth-home.html", ctHTML},
		{"web-flows", nil, "hearth-web-flows.html", ctHTML},
		{"mobile-flow", url.Values{"flowId": {"fl_onboarding"}}, "hearth-onboarding.html", ctHTML},
		{"mobile-flow", url.Values{"flowId": {"fl_account"}}, "hearth-account.html", ctHTML},
		{"draw-svg", nil, "hearth-draw.svg", ctSVG},
		{"design-md", nil, "DESIGN.md", ctMD},
	}
	for _, c := range cases {
		f := gen(t, p, c.kind, c.q)
		if f.Name != c.name || f.ContentType != c.ct {
			t.Errorf("%s: name %q ct %q, want %q %q", c.kind, f.Name, f.ContentType, c.name, c.ct)
		}
	}
	if _, err := Generate(p, "pdf", nil); err == nil || !strings.Contains(err.Error(), "design-md") {
		t.Errorf("unknown kind error should name valid kinds: %v", err)
	}
	if _, err := Generate(p, "page", url.Values{"nodeId": {"sc_home"}}); err == nil {
		t.Error("page export of a screen should fail")
	}
	p.Doc.Markdown = "No heading here"
	if f := gen(t, p, "draw-svg", nil); f.Name != "project-draw.svg" {
		t.Errorf("fallback name %q", f.Name)
	}
}

func TestProjectNameAndSlug(t *testing.T) {
	p := studio.MakeSeed()
	if ProjectName(p) != "Hearth" {
		t.Fatalf("project name %q", ProjectName(p))
	}
	if Slug("Pick your craft") != "pick-your-craft" || Slug("  Hello, World!! ") != "hello-world" {
		t.Fatal("slug")
	}
}

func TestPageHTML(t *testing.T) {
	p := studio.MakeSeed()
	body := string(gen(t, p, "page", url.Values{"nodeId": {"pg_home"}}).Body)
	for _, want := range []string{
		`href="#shop"`, `href="#cart"`, "Everyday ceramics, fired slowly.",
		"@media (max-width:768px)", "@media (max-width:390px)", "fonts.googleapis.com",
		"#D8432A", "Menu", "grid-template-columns:repeat(3", "grid-template-columns:repeat(2", "grid-template-columns:minmax(0,1fr)",
		".t-display{font-size:32px;line-height:38px}", // display → xl at tablet/phone
	} {
		if !strings.Contains(body, want) {
			t.Errorf("home page missing %q", want)
		}
	}
	// blocks in order
	order := []string{`data-block="nav"`, `data-block="hero"`, `data-block="features"`, `data-block="quote"`, `data-block="footer"`}
	last := -1
	for _, o := range order {
		i := strings.Index(body, o)
		if i <= last {
			t.Errorf("block %s out of order", o)
		}
		last = i
	}
	// hero pad 2 (8px) steps to index 3 (16px) on phone
	if !regexp.MustCompile(`@media \(max-width:390px\)\{[^@]*\.k2\{padding:16px 24px\}`).MatchString(body) {
		t.Error("hero phone padding override missing")
	}
	if strings.Contains(body, "<script") {
		t.Error("page export should not need script")
	}
	noPins(t, p, "page", body)
	// token change resolves to hex
	p.Tokens.Colors["accent"] = "#0055FF"
	if !strings.Contains(string(gen(t, p, "page", url.Values{"nodeId": {"pg_home"}}).Body), "#0055FF") {
		t.Error("accent token not resolved")
	}
}

func TestWebFlows(t *testing.T) {
	p := studio.MakeSeed()
	body := string(gen(t, p, "web-flows", nil).Body)
	for _, id := range []string{"home", "shop", "cart", "confirm"} {
		if !strings.Contains(body, `<section class="page" id="`+id+`"`) {
			t.Errorf("missing page section %s", id)
		}
	}
	if !strings.Contains(body, `var START = "home"`) {
		t.Error("should start at Home")
	}
	script := body[strings.Index(body, "<script>"):]
	if n := strings.Count(strings.TrimSpace(script[len("<script>"):strings.Index(script, "</script>")]), "\n") + 1; n > 20 {
		t.Errorf("router script has %d lines", n)
	}
	noPins(t, p, "web-flows", body)
}

func TestMobileFlow(t *testing.T) {
	p := studio.MakeSeed()
	body := string(gen(t, p, "mobile-flow", url.Values{"flowId": {"fl_onboarding"}}).Body)
	for _, id := range []string{"sc_welcome", "sc_craft", "sc_notify", "sc_home", "sc_profile"} {
		if !strings.Contains(body, `data-node="`+id+`"`) {
			t.Errorf("onboarding export missing %s", id)
		}
	}
	if !strings.Contains(body, `var START = "scr-welcome"`) {
		t.Error("should start at Welcome")
	}
	if !strings.Contains(body, `data-to="scr-home"`) || !strings.Contains(body, `class="hs"`) {
		t.Error("cross-flow tap target missing")
	}
	if !strings.Contains(body, "height:36px") { // Profile Sign out is sm
		t.Error("sm CTA height not rendered")
	}
	noPins(t, p, "mobile-flow", body)
	if _, err := Generate(p, "mobile-flow", url.Values{"flowId": {"fl_marketing"}}); err == nil {
		t.Error("web flow should be refused")
	}
}

func TestDrawSVG(t *testing.T) {
	p := studio.MakeSeed()
	body := string(gen(t, p, "draw-svg", nil).Body)
	// bounds: x 120 (rect/text) … 712 (pen); y 68 (text top: 90 − 22) … 376 (pen)
	if !strings.Contains(body, `viewBox="88 36 656 372"`) {
		t.Errorf("viewBox wrong: %s", body[:strings.Index(body, "\n")])
	}
	for _, want := range []string{`<marker id="arrow-ink"`, `marker-end="url(#arrow-ink)"`, "<ellipse", "<polyline", "Hearth — kiln map", "Fraunces", "#D8432A"} {
		if !strings.Contains(body, want) {
			t.Errorf("svg missing %q", want)
		}
	}
	if strings.Contains(body, "pattern") {
		t.Error("svg should have no grid")
	}
	noPins(t, p, "draw-svg", body)
	p.Pins = append(p.Pins, &studio.Pin{ID: "pin_9", N: 9, Kind: "issue", Body: "draw pin body", Target: studio.PinTarget{Type: "draw"}, X: 5000, Y: 5000})
	if b := string(gen(t, p, "draw-svg", nil).Body); strings.Contains(b, "draw pin body") || !strings.Contains(b, `viewBox="88 36 656 372"`) {
		t.Error("draw pins leaked into the SVG or its bounds")
	}
}

func TestDesignMD(t *testing.T) {
	p := studio.MakeSeed()
	md := DesignMD(p)
	if !strings.HasPrefix(md, studio.SeedMarkdown[:len(studio.SeedMarkdown)-1]) {
		t.Error("author markdown not verbatim")
	}
	for _, want := range []string{
		"\n\n---\n\n## Flows (generated)\n\n### Web\n- **Marketing** (start: Home)\n  - Home → Shop — \"Shop\" (primary)\n",
		`  - Home → Cart — "Cart" (cross → Checkout)`,
		"- **Checkout** (start: Cart)",
		"### Mobile\n- **Onboarding** (start: Welcome)\n  - Welcome → Pick your craft — \"Get started\" (primary)\n  - Welcome → Home — \"Skip\" (cross → Account)\n",
		"## Open issues (generated)\n\n| # | Kind | Where | Note |\n|---|------|-------|------|\n",
		"| 1 | Issue | Web › Home › Hero | Body copy is set in `xs` with 8px padding. Reads cramped next to the display headline. |",
		"| 2 | Issue | Web › Shop › CTA band | \"Add to cart\" button is muted-on-wash. Fails AA. |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("DESIGN.md missing:\n%s\n--- got:\n%s", want, md[strings.Index(md, "---"):])
		}
	}
	// edges per flow in lane order: Shop's edges follow Home's
	if i, j := strings.Index(md, "Home → Cart"), strings.Index(md, "Shop → Cart"); i < 0 || j < i {
		t.Error("edge order")
	}
	// multi-line bodies, other targets, resolved pins
	p.Pins = append(p.Pins,
		&studio.Pin{ID: "a", N: 3, Kind: "question", Body: "line one\n  line two", Target: studio.PinTarget{Type: "board", NodeID: "pg_cart"}},
		&studio.Pin{ID: "b", N: 4, Kind: "suggestion", Body: "tap", Target: studio.PinTarget{Type: "node", NodeID: "sc_profile"}},
		&studio.Pin{ID: "c", N: 5, Kind: "issue", Body: "draw", Target: studio.PinTarget{Type: "draw"}})
	md = DesignMD(p)
	for _, want := range []string{"| 3 | Question | Web › Storyboard › Cart | line one line two |", "| 4 | Suggestion | Mobile › Profile | tap |", "| 5 | Issue | Draw | draw |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing row %q", want)
		}
	}
	for _, x := range p.Pins {
		x.Resolved = true
	}
	md = DesignMD(p)
	if !strings.HasSuffix(md, "## Open issues (generated)\n\n_No open issues._\n") {
		t.Errorf("no-issues tail wrong: %q", md[len(md)-80:])
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	p := studio.MakeSeed()
	path, err := Write(dir, "design-md", gen(t, p, "design-md", nil))
	if err != nil || path != filepath.Join(dir, "DESIGN.md") {
		t.Fatalf("design-md path %q err %v", path, err)
	}
	f := gen(t, p, "draw-svg", nil)
	path, err = Write(dir, "draw-svg", f)
	if err != nil || path != filepath.Join(dir, ".agentcanvas", "exports", "hearth-draw.svg") {
		t.Fatalf("svg path %q err %v", path, err)
	}
	// replaces
	f.Body = []byte("<svg/>")
	if _, err := Write(dir, "draw-svg", f); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "<svg/>" {
		t.Error("write did not replace")
	}
	if _, err := Write(dir, "nope", f); err == nil {
		t.Error("unknown kind should fail")
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".agentcanvas", "exports", ".*tmp*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}
