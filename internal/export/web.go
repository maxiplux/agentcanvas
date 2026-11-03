package export

import (
	"fmt"
	"html"
	"strings"

	"agentcanvas/internal/studio"
)

var roleOrder = []string{"xs", "sm", "md", "lg", "xl", "display"}

// webDoc renders web pages into HTML, collecting per-block CSS as it goes.
type webDoc struct {
	p      *studio.Project
	slugs  map[string]string
	blocks strings.Builder // per-block desktop rules
	phone  strings.Builder // per-block phone padding overrides
	n      int
}

func newWebDoc(p *studio.Project) *webDoc {
	return &webDoc{p: p, slugs: nodeSlugs(p, studio.ChanWeb)}
}

func esc(s string) string { return html.EscapeString(s) }

func (d *webDoc) c(key string) string { return color(d.p, key) }

func (d *webDoc) href(to studio.Ref) string {
	if to == "" {
		return "#"
	}
	if s, ok := d.slugs[string(to)]; ok {
		return "#" + s
	}
	return "#"
}

func (d *webDoc) link(l *studio.Link, class, style string) string {
	attr := ""
	if style != "" {
		attr = fmt.Sprintf(` style="%s"`, style)
	}
	return fmt.Sprintf(`<a class="%s" href="%s"%s>%s</a>`, class, d.href(l.ToID), attr, esc(l.Label))
}

func padCSS(px int) string {
	h := px
	if h < 24 {
		h = 24
	}
	return fmt.Sprintf("%dpx %dpx", px, h)
}

// effectivePad mirrors §9.1 (phone: index −1, min 3).
func effectivePad(pad int) int {
	if pad-1 < 3 {
		return 3
	}
	return pad - 1
}

// renderPage writes the blocks of one page.
func (d *webDoc) renderPage(n *studio.Node, out *strings.Builder) {
	for _, b := range n.Blocks {
		d.renderBlock(b, out)
	}
}

func (d *webDoc) renderBlock(b *studio.Block, out *strings.Builder) {
	d.n++
	cls := fmt.Sprintf("k%d", d.n)
	fmt.Fprintf(&d.blocks, ".%s{background:%s;color:%s;padding:%s}\n", cls, d.c(b.Fill), d.c(b.Ink), padCSS(space(d.p, b.Pad)))
	if ep := effectivePad(b.Pad); ep != b.Pad {
		fmt.Fprintf(&d.phone, ".%s{padding:%s}\n", cls, padCSS(space(d.p, ep)))
	}
	gap := space(d.p, b.Gap)
	role := "t-" + b.TypeRole
	weight := fmt.Sprintf("font-weight:%d", b.Weight)
	muted := "color:" + d.c("muted")
	center := b.Align == "center"
	inStyle := fmt.Sprintf("gap:%dpx", gap)
	if center && b.Type != "hero" && b.Type != "nav" {
		inStyle += ";text-align:center"
	}
	fmt.Fprintf(out, `<section class="blk %s %s" data-block="%s"><div class="in" style="%s">`, b.Type, cls, esc(b.Type), inStyle)
	switch b.Type {
	case "nav":
		id := "m" + fmt.Sprint(d.n)
		var links []string
		for _, l := range b.Links {
			links = append(links, d.link(l, role, ""))
		}
		fmt.Fprintf(out, `<input type="checkbox" id="%s" class="menu-cb" aria-hidden="true">`, id)
		fmt.Fprintf(out, `<div class="nav-bar"><span class="%s" style="font-weight:600">%s</span><nav class="nav-links" style="gap:%dpx">%s</nav><label for="%s" class="menu-btn t-sm">Menu</label></div>`,
			role, esc(b.Brand), gap, strings.Join(links, ""), id)
		fmt.Fprintf(out, `<nav class="nav-drop">%s</nav>`, strings.Join(links, ""))
	case "hero":
		col := "hero-col"
		act := "actions"
		if center {
			col += " center"
			act += " center"
		}
		fmt.Fprintf(out, `<div class="%s" style="gap:%dpx">`, col, gap)
		if b.Eyebrow != "" {
			fmt.Fprintf(out, `<p class="eyebrow t-sm" style="%s">%s</p>`, muted, esc(b.Eyebrow))
		}
		fmt.Fprintf(out, `<h1 class="%s" style="%s">%s</h1>`, role, weight, esc(b.Headline))
		if b.Body != "" {
			fmt.Fprintf(out, `<p class="t-%s">%s</p>`, b.BodyRole, esc(b.Body))
		}
		if b.CTA != nil || b.Secondary != nil {
			fmt.Fprintf(out, `<div class="%s">`, act)
			if b.CTA != nil {
				out.WriteString(d.link(b.CTA, "btn t-sm", fmt.Sprintf("background:%s;color:%s;font-weight:600", d.c("accent"), d.c("paper"))))
			}
			if b.Secondary != nil {
				out.WriteString(d.link(b.Secondary, "sec-link t-sm", ""))
			}
			out.WriteString(`</div>`)
		}
		out.WriteString(`</div>`)
	case "features":
		fmt.Fprintf(out, `<h2 class="%s" style="%s">%s</h2><div class="grid" style="gap:%dpx">`, role, weight, esc(b.Heading), gap)
		for _, it := range b.Items {
			fmt.Fprintf(out, `<div class="card" style="border-top-color:%s"><h3 class="t-md" style="font-weight:600">%s</h3><p class="t-sm">%s</p></div>`,
				d.c("line"), esc(it.Title), esc(it.Body))
		}
		out.WriteString(`</div>`)
	case "quote":
		fmt.Fprintf(out, `<blockquote class="%s" style="%s">“%s”</blockquote>`, role, weight, esc(b.Text))
		if b.Cite != "" {
			fmt.Fprintf(out, `<cite class="t-sm" style="%s">— %s</cite>`, muted, esc(b.Cite))
		}
	case "cta":
		fmt.Fprintf(out, `<h2 class="%s" style="%s">%s</h2>`, role, weight, esc(b.Headline))
		if b.Body != "" {
			fmt.Fprintf(out, `<p class="t-md">%s</p>`, esc(b.Body))
		}
		if b.Button != nil {
			act := "actions"
			if center {
				act += " center"
			}
			fmt.Fprintf(out, `<div class="%s">%s</div>`, act,
				d.link(b.Button, "btn t-sm", fmt.Sprintf("background:%s;color:%s;font-weight:600", d.c(b.ButtonFill), d.c(b.ButtonInk))))
		}
	case "footer":
		fmt.Fprintf(out, `<p class="%s" style="font-weight:600">%s</p>`, role, esc(b.Brand))
		if len(b.Links) > 0 {
			var links []string
			for _, l := range b.Links {
				links = append(links, d.link(l, role, ""))
			}
			fmt.Fprintf(out, `<nav class="foot-links" style="gap:%dpx">%s</nav>`, gap, strings.Join(links, ""))
		}
		if b.Note != "" {
			fmt.Fprintf(out, `<p class="t-xs" style="%s">%s</p>`, muted, esc(b.Note))
		}
	}
	out.WriteString("</div></section>\n")
}

func (d *webDoc) typeRule(role, as string) string {
	t := typeSpec(d.p, as)
	return fmt.Sprintf(".t-%s{font-size:%dpx;line-height:%dpx}", role, t.Size, t.LH)
}

// css returns the stylesheet: base, type roles, per-block rules, and the
// 768/390 media queries that mirror §5.4.
func (d *webDoc) css(extra string) string {
	var s strings.Builder
	fmt.Fprintf(&s, "*{box-sizing:border-box}\nbody{margin:0;background:%s;color:%s;font-family:'Fraunces',Georgia,serif;font-optical-sizing:auto;-webkit-font-smoothing:antialiased}\n", d.c("paper"), d.c("ink"))
	s.WriteString("h1,h2,h3,p,blockquote{margin:0}\na{color:inherit}\n")
	for _, r := range roleOrder {
		t := typeSpec(d.p, r)
		fmt.Fprintf(&s, ".t-%s{font-size:%dpx;line-height:%dpx;font-weight:%d}\n", r, t.Size, t.LH, t.Weight)
	}
	s.WriteString(`.blk .in{display:flex;flex-direction:column;max-width:1120px;margin:0 auto}
.nav-bar{display:flex;justify-content:space-between;align-items:center;gap:24px}
.nav-links{display:flex;flex-wrap:wrap;align-items:center}
.nav-links a,.nav-drop a,.foot-links a{text-decoration:none}
.menu-cb{display:none}
.menu-btn{display:none;align-items:center;height:32px;padding:0 12px;border:1px solid currentColor;border-radius:4px;cursor:pointer;font-weight:600}
.nav-drop{display:none;flex-direction:column;gap:8px}
.eyebrow{text-transform:uppercase;letter-spacing:.12em}
.hero-col{display:flex;flex-direction:column;max-width:720px}
.hero-col.center{margin:0 auto;text-align:center;align-items:center}
.actions{display:flex;gap:16px;align-items:center;flex-wrap:wrap}
.actions.center{justify-content:center}
.btn{display:inline-flex;align-items:center;justify-content:center;height:44px;padding:0 20px;border-radius:4px;border:0;text-decoration:none;white-space:nowrap}
.sec-link{text-decoration:underline}
.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr))}
.card{border-top:1px solid;padding-top:12px;display:flex;flex-direction:column;gap:4px}
cite{font-style:normal}
.foot-links{display:flex;flex-wrap:wrap}
`)
	s.WriteString(d.blocks.String())
	s.WriteString(extra)
	// tablet: display→xl, xl→lg; 2 feature columns
	fmt.Fprintf(&s, "@media (max-width:768px){\n%s\n%s\n.grid{grid-template-columns:repeat(2,minmax(0,1fr))}\n}\n",
		d.typeRule("display", "xl"), d.typeRule("xl", "lg"))
	// phone: every role ≥ lg steps down one; 1 column; nav collapses; footer links stack; pad −1 (min 3)
	fmt.Fprintf(&s, "@media (max-width:390px){\n%s\n%s\n%s\n.grid{grid-template-columns:minmax(0,1fr)}\n.nav-bar .nav-links{display:none}\n.menu-btn{display:inline-flex}\n.menu-cb:checked~.nav-drop{display:flex}\n.foot-links{flex-direction:column}\n%s}\n",
		d.typeRule("lg", "md"), d.typeRule("xl", "lg"), d.typeRule("display", "xl"), d.phone.String())
	return s.String()
}

func htmlDoc(title, css, body string) []byte {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n%s\n<style>\n%s</style>\n</head>\n<body>\n%s</body>\n</html>\n", esc(title), fontsLink, css, body)
	return []byte(b.String())
}

func pageFile(p *studio.Project, nodeID string) (*File, error) {
	var n *studio.Node
	if nodeID == "" {
		if n = firstWebNode(p); n == nil {
			return nil, fmt.Errorf("the project has no web pages to export")
		}
	} else {
		n = p.Node(nodeID)
		if n == nil {
			return nil, fmt.Errorf("unknown node id %q", nodeID)
		}
		if n.Kind != "page" {
			return nil, fmt.Errorf("%s is a mobile screen; the page export needs a web page (use mobile-flow for screens)", nodeID)
		}
	}
	d := newWebDoc(p)
	var body strings.Builder
	fmt.Fprintf(&body, "<main id=\"%s\">\n", d.slugs[n.ID])
	d.renderPage(n, &body)
	body.WriteString("</main>\n")
	name := projectSlug(p) + "-" + d.slugs[n.ID] + ".html"
	return &File{Name: name, ContentType: ctHTML, Body: htmlDoc(displayName(p)+" — "+n.Name, d.css(""), body.String())}, nil
}

// webFlowsScript shows one page section at a time from location.hash.
const webFlowsScript = `<script>
(function () {
  var START = %q;
  var pages = document.querySelectorAll('section.page');
  function show() {
    var id = decodeURIComponent(location.hash.slice(1)) || START, found = false;
    pages.forEach(function (p) { if (p.id === id) found = true; });
    if (!found) id = START;
    pages.forEach(function (p) { p.classList.toggle('on', p.id === id); });
    window.scrollTo(0, 0);
  }
  window.addEventListener('hashchange', show);
  show();
})();
</script>
`

func webFlowsFile(p *studio.Project) (*File, error) {
	start := firstWebNode(p)
	if start == nil {
		return nil, fmt.Errorf("the project has no web pages to export")
	}
	d := newWebDoc(p)
	var body strings.Builder
	seen := map[string]bool{}
	emit := func(n *studio.Node) {
		if seen[n.ID] || n.Kind != "page" {
			return
		}
		seen[n.ID] = true
		fmt.Fprintf(&body, "<section class=\"page\" id=\"%s\" data-name=\"%s\">\n", d.slugs[n.ID], esc(n.Name))
		d.renderPage(n, &body)
		body.WriteString("</section>\n")
	}
	for _, f := range p.FlowsOf(studio.ChanWeb) {
		for _, n := range studio.LaneOrder(f, p.Nodes, p.Edges) {
			emit(n)
		}
	}
	for _, n := range p.Nodes {
		emit(n)
	}
	body.WriteString(fmt.Sprintf(webFlowsScript, d.slugs[start.ID]))
	css := d.css("section.page{display:none}\nsection.page.on{display:block}\n")
	name := projectSlug(p) + "-web-flows.html"
	return &File{Name: name, ContentType: ctHTML, Body: htmlDoc(displayName(p)+" — Web flows", css, body.String())}, nil
}
