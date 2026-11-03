package studio

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"
)

// HistoryCap is the per-tab undo depth (§11.3).
const HistoryCap = 50

// CoalesceWindow groups same-key actions into one step (§11.3).
const CoalesceWindow = 600 * time.Millisecond

type history struct {
	past    [][]byte
	future  [][]byte
	openKey string
	lastAt  time.Time
}

func (h *history) push(s []byte) {
	h.past = append(h.past, s)
	if len(h.past) > HistoryCap {
		h.past = h.past[len(h.past)-HistoryCap:]
	}
}

// channelSlice is the web/story snapshot.
type channelSlice struct {
	Flows  []*Flow `json:"flows"`
	Nodes  []*Node `json:"nodes"`
	Edges  []*Edge `json:"edges"`
	Pins   []*Pin  `json:"pins"`
	Tokens Tokens  `json:"tokens"`
}

type drawSlice struct {
	Shapes []*Shape `json:"shapes"`
	Pins   []*Pin   `json:"pins"`
}

type docSlice struct {
	Doc Doc `json:"doc"`
}

func tabChannel(tab string) string {
	if tab == "story" {
		return ChanMobile
	}
	return ChanWeb
}

func pinChannel(p *Project, x *Pin, extra map[string]*Node) string {
	if x.Target.Type == "draw" {
		return "draw"
	}
	if n := p.Node(x.Target.NodeID); n != nil {
		return n.Channel()
	}
	if n := extra[x.Target.NodeID]; n != nil {
		return n.Channel()
	}
	return ""
}

func edgeChannel(nodes map[string]*Node, e *Edge) string {
	if n := nodes[e.FromID]; n != nil {
		return n.Channel()
	}
	return ""
}

func nodeMap(lists ...[]*Node) map[string]*Node {
	m := map[string]*Node{}
	for _, l := range lists {
		for _, n := range l {
			m[n.ID] = n
		}
	}
	return m
}

// sliceJSON serializes the part of p that a tab's history covers.
func sliceJSON(p *Project, tab string) []byte {
	var v any
	switch tab {
	case "web", "story":
		ch := tabChannel(tab)
		s := channelSlice{Flows: []*Flow{}, Nodes: []*Node{}, Edges: []*Edge{}, Pins: []*Pin{}, Tokens: p.Tokens}
		nodes := nodeMap(p.Nodes)
		for _, f := range p.Flows {
			if f.Channel == ch {
				s.Flows = append(s.Flows, f)
			}
		}
		for _, n := range p.Nodes {
			if n.Channel() == ch {
				s.Nodes = append(s.Nodes, n)
			}
		}
		for _, e := range p.Edges {
			if edgeChannel(nodes, e) == ch {
				s.Edges = append(s.Edges, e)
			}
		}
		for _, x := range p.Pins {
			if pinChannel(p, x, nil) == ch {
				s.Pins = append(s.Pins, x)
			}
		}
		v = s
	case "draw":
		s := drawSlice{Shapes: p.Draw.Shapes, Pins: []*Pin{}}
		for _, x := range p.Pins {
			if x.Target.Type == "draw" {
				s.Pins = append(s.Pins, x)
			}
		}
		v = s
	case "doc":
		v = docSlice{Doc: p.Doc}
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// restoreSlice replaces the tab's part of p with a snapshot.
func restoreSlice(p *Project, tab string, data []byte) {
	switch tab {
	case "web", "story":
		ch := tabChannel(tab)
		var s channelSlice
		if err := json.Unmarshal(data, &s); err != nil {
			panic(err)
		}
		extra := nodeMap(s.Nodes)
		allNodes := nodeMap(p.Nodes, s.Nodes)
		var flows []*Flow
		var nodes []*Node
		var edges []*Edge
		// web items come before mobile items
		keepOther := func() {
			for _, f := range p.Flows {
				if f.Channel != ch {
					flows = append(flows, f)
				}
			}
			for _, n := range p.Nodes {
				if n.Channel() != ch {
					nodes = append(nodes, n)
				}
			}
			for _, e := range p.Edges {
				if c := edgeChannel(allNodes, e); c != ch {
					edges = append(edges, e)
				}
			}
		}
		if ch == ChanMobile {
			keepOther()
		}
		flows = append(flows, s.Flows...)
		nodes = append(nodes, s.Nodes...)
		edges = append(edges, s.Edges...)
		if ch == ChanWeb {
			keepOther()
		}
		var pins []*Pin
		for _, x := range p.Pins {
			if pinChannel(p, x, extra) != ch {
				pins = append(pins, x)
			}
		}
		pins = append(pins, s.Pins...)
		sort.SliceStable(pins, func(i, j int) bool { return pins[i].N < pins[j].N })
		p.Flows, p.Nodes, p.Edges, p.Pins = flows, nodes, edges, pins
		p.Tokens = s.Tokens
	case "draw":
		var s drawSlice
		if err := json.Unmarshal(data, &s); err != nil {
			panic(err)
		}
		p.Draw.Shapes = s.Shapes
		var pins []*Pin
		for _, x := range p.Pins {
			if x.Target.Type != "draw" {
				pins = append(pins, x)
			}
		}
		pins = append(pins, s.Pins...)
		sort.SliceStable(pins, func(i, j int) bool { return pins[i].N < pins[j].N })
		p.Pins = pins
	case "doc":
		var s docSlice
		if err := json.Unmarshal(data, &s); err != nil {
			panic(err)
		}
		p.Doc = s.Doc
	}
	p.normalize()
}

// ---- changed ids ----

type idJSON struct {
	ids   []string
	items map[string][]byte
}

func (x *idJSON) add(id string, v any) {
	b, _ := json.Marshal(v)
	if x.items == nil {
		x.items = map[string][]byte{}
	}
	if _, ok := x.items[id]; !ok {
		x.ids = append(x.ids, id)
	}
	x.items[id] = b
}

func index(p *Project) []*idJSON {
	var flows, nodes, children, edges, shapes, pins idJSON
	for _, f := range p.Flows {
		flows.add(f.ID, f)
	}
	for _, n := range p.Nodes {
		nodes.add(n.ID, n)
		for _, b := range n.Blocks {
			children.add(b.ID, b)
		}
		for _, h := range n.Hotspots {
			children.add(h.ID, h)
		}
	}
	for _, e := range p.Edges {
		edges.add(e.ID, e)
	}
	for _, s := range p.Draw.Shapes {
		shapes.add(s.ID, s)
	}
	for _, x := range p.Pins {
		pins.add(x.ID, x)
	}
	var misc idJSON
	misc.add("tokens", p.Tokens)
	misc.add("doc", p.Doc)
	misc.add("draw.view", p.Draw.View)
	misc.add("canvas.view", p.Canvas.View)
	return []*idJSON{&flows, &nodes, &children, &edges, &shapes, &pins, &misc}
}

// ChangedIDs lists ids added, removed, or modified between two projects.
func ChangedIDs(a, b *Project) []string {
	out := []string{}
	seen := map[string]bool{}
	ia, ib := index(a), index(b)
	for i := range ia {
		x, y := ia[i], ib[i]
		for _, id := range y.ids {
			if old, ok := x.items[id]; !ok || !bytes.Equal(old, y.items[id]) {
				if !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
		}
		for _, id := range x.ids {
			if _, ok := y.items[id]; !ok && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}
