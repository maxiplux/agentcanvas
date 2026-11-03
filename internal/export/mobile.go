package export

import (
	"fmt"
	"strings"

	"agentcanvas/internal/studio"
)

const (
	screenW = 390
	screenH = 844
)

// reachableScreens returns the flow's screens in lane order followed by any
// screen (in any flow) reachable from them through edges, in node order.
func reachableScreens(p *studio.Project, f *studio.Flow) []*studio.Node {
	lane := studio.LaneOrder(f, p.Nodes, p.Edges)
	seen := map[string]bool{}
	var queue []string
	for _, n := range lane {
		seen[n.ID] = true
		queue = append(queue, n.ID)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range p.Edges {
			if e.FromID == id && !seen[e.ToID] {
				if n := p.Node(e.ToID); n != nil && n.Kind == "screen" {
					seen[e.ToID] = true
					queue = append(queue, e.ToID)
				}
			}
		}
	}
	out := append([]*studio.Node(nil), lane...)
	inLane := map[string]bool{}
	for _, n := range lane {
		inLane[n.ID] = true
	}
	for _, n := range p.Nodes {
		if seen[n.ID] && !inLane[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

func ctaHeight(size string) int {
	switch size {
	case "sm":
		return 36
	case "lg":
		return 52
	}
	return 44
}

type mobileDoc struct {
	p        *studio.Project
	slugs    map[string]string
	included map[string]bool
}

func (m *mobileDoc) domID(nodeID string) string { return "scr-" + m.slugs[nodeID] }

func (m *mobileDoc) dataTo(to studio.Ref) string {
	if to == "" || !m.included[string(to)] {
		return ""
	}
	return fmt.Sprintf(` data-to="%s"`, m.domID(string(to)))
}

func (m *mobileDoc) font(role string) string {
	t := typeSpec(m.p, role)
	return fmt.Sprintf("font-size:%dpx;line-height:%dpx", t.Size, t.LH)
}

// bars renders the story progress bars of the screen's own flow (lane order).
func (m *mobileDoc) bars(n *studio.Node) string {
	f := m.p.Flow(n.FlowID)
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="bars">`)
	for _, x := range studio.LaneOrder(f, m.p.Nodes, m.p.Edges) {
		cls := "bar"
		if x.ID == n.ID {
			cls += " on"
		}
		fmt.Fprintf(&b, `<i class="%s"></i>`, cls)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (m *mobileDoc) cta(n *studio.Node) string {
	if n.CTA == nil {
		return ""
	}
	pad := space(m.p, n.Pad)
	w := screenW - 2*pad
	return fmt.Sprintf(`<button class="cta" type="button"%s style="width:%dpx;height:%dpx;background:%s;color:%s;%s;font-weight:600">%s</button>`,
		m.dataTo(n.CTA.ToID), w, ctaHeight(n.CTA.Size), color(m.p, n.CTA.Fill), color(m.p, n.CTA.Ink), m.font("lg"), esc(n.CTA.Label))
}

func (m *mobileDoc) texts(n *studio.Node) (title, body string) {
	title = fmt.Sprintf(`<h1 style="%s;font-weight:%d">%s</h1>`, m.font(n.TypeRole), n.Weight, esc(n.Title))
	if n.Body == "" {
		return title, ""
	}
	if n.Archetype == "quote" {
		body = fmt.Sprintf(`<p style="%s;color:%s">%s</p>`, m.font("sm"), color(m.p, "muted"), esc(n.Body))
	} else {
		body = fmt.Sprintf(`<p style="%s">%s</p>`, m.font("md"), esc(n.Body))
	}
	return title, body
}

func (m *mobileDoc) renderScreen(n *studio.Node, out *strings.Builder) {
	p := m.p
	pad := space(p, n.Pad)
	gap := space(p, n.Gap)
	fill, ink := color(p, n.Fill), color(p, n.Ink)
	align := "flex-start"
	textAlign := "left"
	if n.Align == "center" {
		align, textAlign = "center", "center"
	}
	bg := fill
	if n.Archetype == "photo" {
		bg = fmt.Sprintf("repeating-linear-gradient(45deg,%s 0 14px,%s 14px 28px)", color(p, "wash"), color(p, "line"))
	}
	fmt.Fprintf(out, `<div class="screen" id="%s" data-node="%s" style="background:%s;color:%s">`, m.domID(n.ID), esc(n.ID), bg, ink)
	if n.Archetype == "photo" {
		out.WriteString(`<div class="scrim"></div>`)
	}
	title, body := m.texts(n)
	cols := fmt.Sprintf("gap:%dpx;text-align:%s;align-items:%s", gap, textAlign, align)
	switch {
	case n.Archetype == "split" || n.Layout == "split":
		half := screenH / 2
		fmt.Fprintf(out, `<div class="half" style="top:0;height:%dpx;background:%s;padding:72px %dpx %dpx;justify-content:flex-end;%s">%s</div>`,
			half, color(p, "wash"), pad, pad, cols, title)
		fmt.Fprintf(out, `<div class="half" style="top:%dpx;height:%dpx;background:%s;padding:%dpx %dpx %dpx;%s"><div class="grow" style="display:flex;flex-direction:column;%s">%s</div>%s</div>`,
			half, screenH-half, fill, pad, pad, pad+24, cols, cols, body, m.cta(n))
	default:
		justify := "center"
		switch {
		case n.Archetype == "cta":
			justify = "flex-start"
		case n.Archetype == "photo":
			justify = "flex-end"
		case n.Layout == "bottom":
			justify = "flex-end"
		}
		fmt.Fprintf(out, `<div class="col" style="padding:72px %dpx %dpx;gap:%dpx;align-items:%s">`, pad, pad+24, gap, align)
		fmt.Fprintf(out, `<div class="grow" style="justify-content:%s;%s">%s%s</div>%s</div>`, justify, cols, title, body, m.cta(n))
	}
	out.WriteString(`<div class="status"><span>9:41</span><span class="glyphs"><i></i><i></i><i></i><b></b></span></div>`)
	out.WriteString(m.bars(n))
	for _, h := range n.Hotspots {
		fmt.Fprintf(out, `<a class="hs" href="#" aria-label="%s"%s style="left:%spx;top:%spx;width:%spx;height:%spx"></a>`,
			esc(h.Label), m.dataTo(h.ToID), num(h.X), num(h.Y), num(h.W), num(h.H))
	}
	out.WriteString(`<div class="home"></div></div>` + "\n")
}

const mobileCSS = `*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#1A1917;font-family:'Fraunces',Georgia,serif;font-optical-sizing:auto;-webkit-font-smoothing:antialiased}
h1,p{margin:0}
.phone{padding:24px;border-radius:40px;background:#0E0D0C;box-shadow:0 30px 80px rgba(0,0,0,.45)}
.area{position:relative;width:390px;height:844px;overflow:hidden;border-radius:18px}
.screen{position:absolute;inset:0;opacity:0;pointer-events:none;transition:opacity .15s ease}
.screen.on{opacity:1;pointer-events:auto}
.scrim{position:absolute;inset:0;background:linear-gradient(to bottom,rgba(0,0,0,0) 35%,rgba(0,0,0,.7))}
.col{position:absolute;inset:0;display:flex;flex-direction:column}
.half{position:absolute;left:0;right:0;display:flex;flex-direction:column}
.grow{flex:1;display:flex;flex-direction:column}
.cta{display:inline-flex;align-items:center;justify-content:center;flex:none;border:0;border-radius:4px;font-family:inherit;cursor:pointer}
.status{position:absolute;top:14px;left:28px;right:28px;display:flex;justify-content:space-between;align-items:center;font:600 15px/1 -apple-system,system-ui,sans-serif;pointer-events:none}
.glyphs{display:flex;gap:3px;align-items:flex-end}
.glyphs i{display:block;width:3px;background:currentColor;border-radius:1px}
.glyphs i:nth-child(1){height:5px}.glyphs i:nth-child(2){height:8px}.glyphs i:nth-child(3){height:11px}
.glyphs b{display:block;width:22px;height:11px;margin-left:6px;border:1.5px solid currentColor;border-radius:3px}
.bars{position:absolute;top:44px;left:16px;right:16px;display:flex;gap:4px;pointer-events:none}
.bar{flex:1;height:3px;border-radius:2px;background:currentColor;opacity:.25}
.bar.on{opacity:1}
.hs{position:absolute;z-index:5;display:block}
.home{position:absolute;bottom:8px;left:50%;width:134px;height:5px;margin-left:-67px;border-radius:3px;background:currentColor;opacity:.6;pointer-events:none}
`

// mobileScript follows data-to on taps; left 25% outside a target goes back.
const mobileScript = `<script>
(function () {
  var START = %q, cur = null, hist = [];
  var area = document.querySelector('.area');
  function go(id) {
    var ok = false;
    document.querySelectorAll('.screen').forEach(function (s) { var on = s.id === id; s.classList.toggle('on', on); ok = ok || on; });
    if (ok) cur = id;
    return ok;
  }
  area.addEventListener('click', function (e) {
    e.preventDefault();
    var t = e.target.closest('[data-to]');
    if (t) { var prev = cur; if (go(t.getAttribute('data-to'))) hist.push(prev); return; }
    var r = area.getBoundingClientRect();
    if (e.clientX - r.left < r.width * 0.25 && hist.length) go(hist.pop());
  });
  document.addEventListener('keydown', function (e) { if (e.key === 'ArrowLeft' && hist.length) go(hist.pop()); });
  go(START);
})();
</script>
`

func mobileFlowFile(p *studio.Project, flowID string) (*File, error) {
	var f *studio.Flow
	if flowID == "" {
		flows := p.FlowsOf(studio.ChanMobile)
		if len(flows) == 0 {
			return nil, fmt.Errorf("the project has no mobile flows to export")
		}
		f = flows[0]
	} else {
		f = p.Flow(flowID)
		if f == nil {
			return nil, fmt.Errorf("unknown flow id %q", flowID)
		}
		if f.Channel != studio.ChanMobile {
			return nil, fmt.Errorf("%s is a web flow; mobile-flow needs a mobile flow (use web-flows for web)", flowID)
		}
	}
	screens := reachableScreens(p, f)
	if len(screens) == 0 {
		return nil, fmt.Errorf("flow %s has no screens", f.Name)
	}
	m := &mobileDoc{p: p, slugs: nodeSlugs(p, studio.ChanMobile), included: map[string]bool{}}
	for _, n := range screens {
		m.included[n.ID] = true
	}
	start := screens[0].ID
	if f.StartID != "" && m.included[string(f.StartID)] {
		start = string(f.StartID)
	}
	var body strings.Builder
	body.WriteString("<div class=\"phone\"><div class=\"area\">\n")
	for _, n := range screens {
		m.renderScreen(n, &body)
	}
	body.WriteString("</div></div>\n")
	body.WriteString(fmt.Sprintf(mobileScript, m.domID(start)))
	fslug := Slug(f.Name)
	if fslug == "" {
		fslug = "flow"
	}
	name := projectSlug(p) + "-" + fslug + ".html"
	return &File{Name: name, ContentType: ctHTML, Body: htmlDoc(displayName(p)+" — "+f.Name, mobileCSS, body.String())}, nil
}
