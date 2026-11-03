package studio

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Breakpoints (§5.4).
var Breakpoints = map[string]int{"desktop": 1280, "tablet": 768, "phone": 390}

func roleIndex(role string) int {
	for i, r := range TypeKeys {
		if r == role {
			return i
		}
	}
	return 2
}

// EffectiveRole implements §9.1.
func EffectiveRole(role, bp string) string {
	i := roleIndex(role)
	if bp == "tablet" && i >= 4 {
		return TypeKeys[i-1]
	}
	if bp == "phone" && i >= 3 {
		return TypeKeys[i-1]
	}
	return role
}

// EffectivePad implements §9.1.
func EffectivePad(pad int, bp string) int {
	if bp == "phone" {
		if pad-1 < 3 {
			return 3
		}
		return pad - 1
	}
	return pad
}

// StepDown moves n steps toward xs and clamps.
func StepDown(role string, n int) string {
	i := roleIndex(role) - n
	if i < 0 {
		i = 0
	}
	return TypeKeys[i]
}

func parseHex(hex string) (r, g, b float64, ok bool) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff), true
}

func channelLin(c float64) float64 {
	c /= 255
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// Luminance is WCAG 2.1 relative luminance.
func Luminance(hex string) float64 {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return 0
	}
	return 0.2126*channelLin(r) + 0.7152*channelLin(g) + 0.0722*channelLin(b)
}

// Contrast returns the WCAG contrast ratio of two hex colors.
func Contrast(a, b string) float64 {
	l1, l2 := Luminance(a), Luminance(b)
	if l2 > l1 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// PassesAA uses 3.0 for large roles and 4.5 otherwise (§9.2).
func PassesAA(ratio float64, role string) bool {
	if role == "lg" || role == "xl" || role == "display" {
		return ratio >= 3
	}
	return ratio >= 4.5
}

// LaneOrder implements §9.5.
func LaneOrder(flow *Flow, nodes []*Node, edges []*Edge) []*Node {
	var inFlow []*Node
	byID := map[string]*Node{}
	for _, n := range nodes {
		if n.FlowID == flow.ID {
			inFlow = append(inFlow, n)
			byID[n.ID] = n
		}
	}
	seen := map[string]bool{}
	var out []*Node
	var q []string
	if flow.StartID != "" {
		q = append(q, string(flow.StartID))
	}
	for len(q) > 0 {
		id := q[0]
		q = q[1:]
		if seen[id] {
			continue
		}
		n := byID[id]
		if n == nil {
			continue
		}
		seen[id] = true
		out = append(out, n)
		for _, e := range edges {
			if e.FromID == id && e.Kind != "back" {
				q = append(q, e.ToID)
			}
		}
	}
	for _, n := range inFlow {
		if !seen[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

// ---- checks (§9.2, §9.4) ----

type ContrastCheck struct {
	NodeID  string  `json:"nodeId"`
	BlockID string  `json:"blockId,omitempty"`
	Part    string  `json:"part"`
	Ratio   float64 `json:"ratio"`
	Role    string  `json:"role"`
	Pass    bool    `json:"pass"`
	Label   string  `json:"label"`
}

type TapCheck struct {
	NodeID string  `json:"nodeId"`
	Ref    string  `json:"ref"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Pass   bool    `json:"pass"`
	Label  string  `json:"label"`
}

type Checks struct {
	Contrast []ContrastCheck `json:"contrast"`
	Tap      []TapCheck      `json:"tap"`
}

// BlockLabel is the human name of a block type.
func BlockLabel(t string) string {
	switch t {
	case "nav":
		return "Nav"
	case "hero":
		return "Hero"
	case "features":
		return "Features"
	case "quote":
		return "Quote"
	case "cta":
		return "CTA band"
	case "footer":
		return "Footer"
	}
	return t
}

// BodyRole is the role of the text a block's body contrast readout measures.
// SPEC: hero body uses bodyRole, feature cards use sm, the CTA band body uses md,
// every other block uses its typeRole.
func BodyRole(b *Block) string {
	switch b.Type {
	case "hero":
		return b.BodyRole
	case "features":
		return "sm"
	case "cta":
		return "md"
	}
	return b.TypeRole
}

// CTAHeight maps mobile CTA sizes to px (§8.4).
func CTAHeight(size string) float64 {
	switch size {
	case "sm":
		return 36
	case "lg":
		return 52
	}
	return 44
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func (p *Project) color(key string) string {
	if c, ok := p.Tokens.Colors[key]; ok {
		return c
	}
	return "#000000"
}

// ComputeChecks returns every checked item at breakpoint bp.
func ComputeChecks(p *Project, bp string) Checks {
	c := Checks{Contrast: []ContrastCheck{}, Tap: []TapCheck{}}
	for _, n := range p.Nodes {
		if n.Kind == "page" {
			for _, b := range n.Blocks {
				role := EffectiveRole(BodyRole(b), bp)
				r := Contrast(p.color(b.Fill), p.color(b.Ink))
				c.Contrast = append(c.Contrast, ContrastCheck{NodeID: n.ID, BlockID: b.ID, Part: "body", Ratio: round1(r), Role: role,
					Pass: PassesAA(r, role), Label: fmt.Sprintf("%s › %s", n.Name, BlockLabel(b.Type))})
				if b.Type == "cta" {
					// Buttons are set in sm (§8.3), so the button check never steps down.
					r := Contrast(p.color(b.ButtonFill), p.color(b.ButtonInk))
					c.Contrast = append(c.Contrast, ContrastCheck{NodeID: n.ID, BlockID: b.ID, Part: "button", Ratio: round1(r), Role: "sm",
						Pass: PassesAA(r, "sm"), Label: fmt.Sprintf("%s › %s button", n.Name, BlockLabel(b.Type))})
				}
			}
			continue
		}
		// mobile screen
		r := Contrast(p.color(n.Fill), p.color(n.Ink))
		c.Contrast = append(c.Contrast, ContrastCheck{NodeID: n.ID, Part: "body", Ratio: round1(r), Role: "md",
			Pass: PassesAA(r, "md"), Label: n.Name})
		if n.CTA != nil {
			// SPEC: mobile CTA labels render in lg weight 600 (large text), so they use the 3.0 threshold.
			r := Contrast(p.color(n.CTA.Fill), p.color(n.CTA.Ink))
			c.Contrast = append(c.Contrast, ContrastCheck{NodeID: n.ID, Part: "cta", Ratio: round1(r), Role: "lg",
				Pass: PassesAA(r, "lg"), Label: fmt.Sprintf("%s › %s", n.Name, n.CTA.Label)})
			pad := n.Pad
			if pad < 0 || pad > 7 {
				pad = 4
			}
			w := float64(390 - 2*SpaceScale[pad])
			h := CTAHeight(n.CTA.Size)
			c.Tap = append(c.Tap, tapCheck(n, "cta", n.CTA.Label, w, h))
		}
		for _, hs := range n.Hotspots {
			c.Tap = append(c.Tap, tapCheck(n, hs.ID, hs.Label, hs.W, hs.H))
		}
	}
	return c
}

func tapCheck(n *Node, ref, label string, w, h float64) TapCheck {
	pass := w >= 44 && h >= 44
	var l string
	switch {
	case h < 44:
		l = fmt.Sprintf("%s › %s: %gpx (min 44)", n.Name, label, h)
	case w < 44:
		l = fmt.Sprintf("%s › %s: %gpx wide (min 44)", n.Name, label, w)
	default:
		l = fmt.Sprintf("%s › %s: %gpx", n.Name, label, h)
	}
	return TapCheck{NodeID: n.ID, Ref: ref, W: w, H: h, Pass: pass, Label: l}
}

// Failing returns only failing checks.
func (c Checks) Failing() Checks {
	out := Checks{Contrast: []ContrastCheck{}, Tap: []TapCheck{}}
	for _, x := range c.Contrast {
		if !x.Pass {
			out.Contrast = append(out.Contrast, x)
		}
	}
	for _, x := range c.Tap {
		if !x.Pass {
			out.Tap = append(out.Tap, x)
		}
	}
	return out
}

// OpenIssues counts unresolved issue pins (§8.1).
func OpenIssues(p *Project) int {
	n := 0
	for _, x := range p.Pins {
		if x.Kind == "issue" && !x.Resolved {
			n++
		}
	}
	return n
}
