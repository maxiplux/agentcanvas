package export

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"agentcanvas/internal/studio"
)

const svgMargin = 32

type bounds struct {
	minX, minY, maxX, maxY float64
	ok                     bool
}

func (b *bounds) add(x, y float64) {
	if !b.ok {
		b.minX, b.maxX, b.minY, b.maxY, b.ok = x, x, y, y, true
		return
	}
	b.minX = math.Min(b.minX, x)
	b.maxX = math.Max(b.maxX, x)
	b.minY = math.Min(b.minY, y)
	b.maxY = math.Max(b.maxY, y)
}

// textWidth estimates rendered width for bounds (Fraunces averages ~0.56em per glyph).
func textWidth(s string, size int) float64 {
	return 0.56 * float64(size) * float64(utf8.RuneCountInString(s))
}

// ShapeBounds returns the content bounds of the draw shapes. Text (x, y) is the
// SVG baseline: the box spans one font size above it and 0.3 em below.
func ShapeBounds(p *studio.Project) (minX, minY, maxX, maxY float64, ok bool) {
	var b bounds
	for _, s := range p.Draw.Shapes {
		switch s.Type {
		case "rect", "ellipse":
			b.add(s.X, s.Y)
			b.add(s.X+s.W, s.Y+s.H)
		case "line", "arrow":
			b.add(s.X1, s.Y1)
			b.add(s.X2, s.Y2)
		case "text":
			t := typeSpec(p, s.Role)
			b.add(s.X, s.Y-float64(t.Size))
			b.add(s.X+textWidth(s.Text, t.Size), s.Y+0.3*float64(t.Size))
		case "pen":
			for _, pt := range s.Points {
				b.add(pt[0], pt[1])
			}
		}
	}
	return b.minX, b.minY, b.maxX, b.maxY, b.ok
}

func drawFile(p *studio.Project) *File {
	minX, minY, maxX, maxY, ok := ShapeBounds(p)
	if !ok {
		minX, minY, maxX, maxY = 0, 0, 0, 0
	}
	vx, vy := minX-svgMargin, minY-svgMargin
	vw, vh := maxX-minX+2*svgMargin, maxY-minY+2*svgMargin
	var s strings.Builder
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" width="%s" height="%s">`+"\n",
		num(vx), num(vy), num(vw), num(vh), num(vw), num(vh))
	fmt.Fprintf(&s, "<title>%s — Draw</title>\n", esc(displayName(p)))
	// one arrowhead marker per stroke token used by arrows
	used := map[string]bool{}
	var keys []string
	for _, sh := range p.Draw.Shapes {
		if sh.Type == "arrow" && !used[sh.Stroke] {
			used[sh.Stroke] = true
			keys = append(keys, sh.Stroke)
		}
	}
	if len(keys) > 0 {
		s.WriteString("<defs>\n")
		for _, k := range keys {
			fmt.Fprintf(&s, `<marker id="arrow-%s" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="%s"/></marker>`+"\n", esc(k), color(p, k))
		}
		s.WriteString("</defs>\n")
	}
	fmt.Fprintf(&s, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n", num(vx), num(vy), num(vw), num(vh), color(p, "paper"))
	for _, sh := range p.Draw.Shapes {
		fill := "none"
		if sh.Fill != "" && sh.Fill != "none" {
			fill = color(p, sh.Fill)
		}
		switch sh.Type {
		case "rect":
			fmt.Fprintf(&s, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s" stroke="%s" stroke-width="%s"/>`+"\n",
				num(sh.X), num(sh.Y), num(sh.W), num(sh.H), fill, color(p, sh.Stroke), num(sh.SW))
		case "ellipse":
			fmt.Fprintf(&s, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s" fill="%s" stroke="%s" stroke-width="%s"/>`+"\n",
				num(sh.X+sh.W/2), num(sh.Y+sh.H/2), num(sh.W/2), num(sh.H/2), fill, color(p, sh.Stroke), num(sh.SW))
		case "line", "arrow":
			marker := ""
			if sh.Type == "arrow" {
				marker = fmt.Sprintf(` marker-end="url(#arrow-%s)"`, esc(sh.Stroke))
			}
			fmt.Fprintf(&s, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round"%s/>`+"\n",
				num(sh.X1), num(sh.Y1), num(sh.X2), num(sh.Y2), color(p, sh.Stroke), num(sh.SW), marker)
		case "text":
			t := typeSpec(p, sh.Role)
			fmt.Fprintf(&s, `<text x="%s" y="%s" font-family="Fraunces, Georgia, serif" font-size="%d" font-weight="%d" fill="%s">%s</text>`+"\n",
				num(sh.X), num(sh.Y), t.Size, t.Weight, color(p, sh.Ink), esc(sh.Text))
		case "pen":
			var pts []string
			for _, pt := range sh.Points {
				pts = append(pts, num(pt[0])+","+num(pt[1]))
			}
			fmt.Fprintf(&s, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"/>`+"\n",
				strings.Join(pts, " "), color(p, sh.Stroke), num(sh.SW))
		}
	}
	s.WriteString("</svg>\n")
	return &File{Name: projectSlug(p) + "-draw.svg", ContentType: ctSVG, Body: []byte(s.String())}
}
