package export

import (
	"fmt"
	"sort"
	"strings"

	"agentcanvas/internal/studio"
)

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func channelTitle(ch string) string {
	if ch == studio.ChanMobile {
		return "Mobile"
	}
	return "Web"
}

// pinWhere is the Open-issues "Where" cell: channel prefix + studio.PinWhere.
func pinWhere(p *studio.Project, x *studio.Pin) string {
	if x.Target.Type == "draw" {
		return "Draw"
	}
	where := studio.PinWhere(p, x)
	if n := p.Node(x.Target.NodeID); n != nil {
		return channelTitle(n.Channel()) + " › " + where
	}
	return where
}

// oneLine joins a multi-line body with single spaces and escapes table pipes.
func oneLine(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.ReplaceAll(strings.Join(parts, " "), "|", `\|`)
}

func nodeName(p *studio.Project, id string) string {
	if n := p.Node(id); n != nil {
		return n.Name
	}
	return id
}

// DesignMD renders the §11.5 DESIGN.md: author markdown verbatim, then the
// generated Flows and Open issues appendices.
func DesignMD(p *studio.Project) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(p.Doc.Markdown, "\n\r\t "))
	b.WriteString("\n\n---\n\n## Flows (generated)\n")
	for _, ch := range []string{studio.ChanWeb, studio.ChanMobile} {
		fmt.Fprintf(&b, "\n### %s\n", channelTitle(ch))
		flows := p.FlowsOf(ch)
		if len(flows) == 0 {
			b.WriteString("_No flows._\n")
			continue
		}
		for _, f := range flows {
			start := "none"
			if f.StartID != "" {
				start = nodeName(p, string(f.StartID))
			}
			fmt.Fprintf(&b, "- **%s** (start: %s)\n", f.Name, start)
			for _, n := range studio.LaneOrder(f, p.Nodes, p.Edges) {
				for _, e := range p.Edges {
					if e.FromID != n.ID {
						continue
					}
					kind := e.Kind
					if e.Kind == "cross" {
						target := ""
						if tn := p.Node(e.ToID); tn != nil {
							if tf := p.Flow(tn.FlowID); tf != nil {
								target = tf.Name
							}
						}
						kind = "cross → " + target
					}
					fmt.Fprintf(&b, "  - %s → %s — \"%s\" (%s)\n", n.Name, nodeName(p, e.ToID), e.Label, kind)
				}
			}
		}
	}
	b.WriteString("\n## Open issues (generated)\n\n")
	var open []*studio.Pin
	for _, x := range p.Pins {
		if !x.Resolved {
			open = append(open, x)
		}
	}
	sort.SliceStable(open, func(i, j int) bool { return open[i].N < open[j].N })
	if len(open) == 0 {
		b.WriteString("_No open issues._\n")
		return b.String()
	}
	b.WriteString("| # | Kind | Where | Note |\n|---|------|-------|------|\n")
	for _, x := range open {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", x.N, capitalize(x.Kind), pinWhere(p, x), oneLine(x.Body))
	}
	return b.String()
}
