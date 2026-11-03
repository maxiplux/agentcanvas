package studio

import (
	"strings"
)

type CanvasChild struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	ToID  Ref    `json:"toId,omitempty"`
}

type CanvasBlock struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Text     string        `json:"text"` // headline, heading, brand, or quote text
	Children []CanvasChild `json:"children"`
}

type CanvasNode struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	FlowID    string        `json:"flowId"`
	Name      string        `json:"name"`
	Blocks    []CanvasBlock `json:"blocks,omitempty"`
	Archetype string        `json:"archetype,omitempty"`
	Title     string        `json:"title,omitempty"`
	CTA       *ScreenCTA    `json:"cta,omitempty"`
	Hotspots  []*Hotspot    `json:"hotspots,omitempty"`
}

type CanvasFlow struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Channel string   `json:"channel"`
	StartID Ref      `json:"startId"`
	NodeIDs []string `json:"nodeIds"` // lane order (§9.5)
}

type CanvasPin struct {
	ID       string    `json:"id"`
	N        int       `json:"n"`
	Kind     string    `json:"kind"`
	Body     string    `json:"body"`
	Resolved bool      `json:"resolved"`
	Target   PinTarget `json:"target"`
	Where    string    `json:"where"`
}

type DocHeading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

// Canvas is the get_canvas result: the agent's screen substitute.
type Canvas struct {
	Rev        int64                   `json:"rev"`
	View       SharedView              `json:"view"`
	History    map[string]HistoryFlags `json:"history"`
	Tokens     Tokens                  `json:"tokens"`
	Flows      []CanvasFlow            `json:"flows"`
	Nodes      []CanvasNode            `json:"nodes"`
	Edges      []*Edge                 `json:"edges"`
	DocHeads   []DocHeading            `json:"docHeadings"`
	Pins       []CanvasPin             `json:"pins"`
	Checks     Checks                  `json:"checks"` // failures only
	OpenIssues int                     `json:"openIssues"`
	Skills     []string                `json:"skills"`
	Counts     map[string]int          `json:"counts"`
	Project    *Project                `json:"project,omitempty"`
}

// DocHeadings lists #, ##, ### headings with 0-based line numbers.
func DocHeadings(md string) []DocHeading {
	out := []DocHeading{}
	for i, line := range strings.Split(md, "\n") {
		lvl := 0
		for lvl < len(line) && lvl < 4 && line[lvl] == '#' {
			lvl++
		}
		if lvl >= 1 && lvl <= 3 && len(line) > lvl && line[lvl] == ' ' {
			out = append(out, DocHeading{Level: lvl, Text: strings.TrimSpace(line[lvl+1:]), Line: i})
		}
	}
	return out
}

func blockText(b *Block) string {
	switch b.Type {
	case "hero", "cta":
		return b.Headline
	case "features":
		return b.Heading
	case "quote":
		return b.Text
	case "nav", "footer":
		return b.Brand
	}
	return ""
}

func canvasNode(n *Node) CanvasNode {
	cn := CanvasNode{ID: n.ID, Kind: n.Kind, FlowID: n.FlowID, Name: n.Name}
	if n.Kind == "page" {
		cn.Blocks = []CanvasBlock{}
		for _, b := range n.Blocks {
			cb := CanvasBlock{ID: b.ID, Type: b.Type, Text: blockText(b), Children: []CanvasChild{}}
			for _, l := range b.Links {
				cb.Children = append(cb.Children, CanvasChild{ID: l.ID, Label: l.Label, ToID: l.ToID})
			}
			for _, it := range b.Items {
				cb.Children = append(cb.Children, CanvasChild{ID: it.ID, Label: it.Title})
			}
			for _, l := range []*Link{b.CTA, b.Secondary, b.Button} {
				if l != nil && (b.Type == "hero" || b.Type == "cta") && l.ID != "" {
					cb.Children = append(cb.Children, CanvasChild{ID: l.ID, Label: l.Label, ToID: l.ToID})
				}
			}
			cn.Blocks = append(cn.Blocks, cb)
		}
		return cn
	}
	cn.Archetype, cn.Title, cn.CTA = n.Archetype, n.Title, n.CTA
	cn.Hotspots = n.Hotspots
	return cn
}

// Canvas builds the get_canvas summary.
func (s *Store) Canvas(full bool) *Canvas {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.project
	c := &Canvas{
		Rev:        s.rev,
		View:       s.view,
		History:    s.historyFlags(),
		Tokens:     p.Tokens,
		Flows:      []CanvasFlow{},
		Nodes:      []CanvasNode{},
		Edges:      p.Edges,
		DocHeads:   DocHeadings(p.Doc.Markdown),
		Pins:       []CanvasPin{},
		Checks:     ComputeChecks(p, s.view.Breakpoint).Failing(),
		OpenIssues: OpenIssues(p),
		Skills:     []string{},
	}
	for _, f := range p.Flows {
		cf := CanvasFlow{ID: f.ID, Name: f.Name, Channel: f.Channel, StartID: f.StartID, NodeIDs: []string{}}
		for _, n := range LaneOrder(f, p.Nodes, p.Edges) {
			cf.NodeIDs = append(cf.NodeIDs, n.ID)
		}
		c.Flows = append(c.Flows, cf)
	}
	for _, n := range p.Nodes {
		c.Nodes = append(c.Nodes, canvasNode(n))
	}
	for _, x := range p.Pins {
		c.Pins = append(c.Pins, CanvasPin{ID: x.ID, N: x.N, Kind: x.Kind, Body: x.Body, Resolved: x.Resolved, Target: x.Target, Where: PinWhere(p, x)})
	}
	for _, sk := range s.skillList() {
		c.Skills = append(c.Skills, sk.Name)
	}
	c.Counts = map[string]int{"flows": len(p.Flows), "nodes": len(p.Nodes), "edges": len(p.Edges), "pins": len(p.Pins), "shapes": len(p.Draw.Shapes)}
	if full {
		c.Project = p.Clone()
	}
	return c
}

// NodeDetail is the get_node result.
type NodeDetail struct {
	Rev  int64   `json:"rev"`
	Node *Node   `json:"node"`
	Flow string  `json:"flow"`
	In   []*Edge `json:"in"`
	Out  []*Edge `json:"out"`
	Pins []*Pin  `json:"pins"`
}

// NodeDetail returns a node with its edges and pins, or an ActionError.
func (s *Store) NodeDetail(id string) (*NodeDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.project
	n := p.Node(id)
	if n == nil {
		return nil, errUnknown("node", id)
	}
	d := &NodeDetail{Rev: s.rev, Node: n, In: []*Edge{}, Out: []*Edge{}, Pins: []*Pin{}}
	if f := p.Flow(n.FlowID); f != nil {
		d.Flow = f.Name
	}
	for _, e := range p.Edges {
		if e.ToID == id {
			d.In = append(d.In, e)
		}
		if e.FromID == id {
			d.Out = append(d.Out, e)
		}
	}
	for _, x := range p.Pins {
		if x.Target.NodeID == id {
			d.Pins = append(d.Pins, x)
		}
	}
	return d, nil
}

// Doc returns the DESIGN.md markdown.
func (s *Store) Doc() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.project.Doc.Markdown
}
