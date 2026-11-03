package studio

import (
	"fmt"
)

// linkRef points at one link-like owner on a node: a block link, a hero CTA or
// secondary, a CTA-band button, a screen CTA, or a hotspot.
type linkRef struct {
	key   string // owner key: "<blockId>/<childId>", "<blockId>/cta", "cta", "<hotspotId>"
	id    string // the link/hotspot id, or "cta" for the screen CTA
	block *Block
	label *string
	toID  *Ref
	name  string // human path for summaries, e.g. "Nav › Cart"
}

// nodeLinks lists every link-like owner on a node.
func nodeLinks(n *Node) []linkRef {
	var out []linkRef
	if n.Kind == "page" {
		for _, b := range n.Blocks {
			bl := BlockLabel(b.Type)
			for _, l := range b.Links {
				out = append(out, linkRef{key: b.ID + "/" + l.ID, id: l.ID, block: b, label: &l.Label, toID: &l.ToID, name: bl + " › " + l.Label})
			}
			if b.Type == "hero" && b.CTA != nil {
				out = append(out, linkRef{key: b.ID + "/cta", id: b.CTA.ID, block: b, label: &b.CTA.Label, toID: &b.CTA.ToID, name: bl + " › " + b.CTA.Label})
			}
			if b.Type == "hero" && b.Secondary != nil {
				out = append(out, linkRef{key: b.ID + "/secondary", id: b.Secondary.ID, block: b, label: &b.Secondary.Label, toID: &b.Secondary.ToID, name: bl + " › " + b.Secondary.Label})
			}
			if b.Type == "cta" && b.Button != nil {
				out = append(out, linkRef{key: b.ID + "/button", id: b.Button.ID, block: b, label: &b.Button.Label, toID: &b.Button.ToID, name: bl + " › " + b.Button.Label})
			}
		}
		return out
	}
	if n.CTA != nil {
		out = append(out, linkRef{key: "cta", id: "cta", label: &n.CTA.Label, toID: &n.CTA.ToID, name: n.CTA.Label})
	}
	for _, h := range n.Hotspots {
		out = append(out, linkRef{key: h.ID, id: h.ID, label: &h.Label, toID: &h.ToID, name: h.Label})
	}
	return out
}

func blockLinks(n *Node, b *Block) []linkRef {
	var out []linkRef
	for _, r := range nodeLinks(n) {
		if r.block == b {
			out = append(out, r)
		}
	}
	return out
}

// edgeKind returns the kind an edge between two nodes must have when not
// chosen by the designer: cross iff different flows.
func edgeKind(from, to *Node) string {
	if from.FlowID != to.FlowID {
		return "cross"
	}
	return "primary"
}

func (r *reducer) findEdge(from, to, label string) *Edge {
	for _, e := range r.p.Edges {
		if e.FromID == from && e.ToID == to && e.Label == label {
			return e
		}
	}
	return nil
}

// hasOwner reports whether any link on node `from` currently targets `to` with `label`.
// SPEC: two links with the same (owner node, toId, label) share one edge (§6.1 allows
// only one edge per triple); the edge is removed only when no link still owns it.
func (r *reducer) hasOwner(from, to, label string) bool {
	n := r.p.Node(from)
	if n == nil {
		return false
	}
	for _, l := range nodeLinks(n) {
		if string(*l.toID) == to && *l.label == label {
			return true
		}
	}
	return false
}

// edgeOwners lists the links whose (owner, toId, label) match the edge.
func (r *reducer) edgeOwners(e *Edge) []linkRef {
	n := r.p.Node(e.FromID)
	if n == nil {
		return nil
	}
	var out []linkRef
	for _, l := range nodeLinks(n) {
		if string(*l.toID) == e.ToID && *l.label == e.Label {
			out = append(out, l)
		}
	}
	return out
}

func (r *reducer) ensureEdge(from, to *Node, label string) *Edge {
	if e := r.findEdge(from.ID, to.ID, label); e != nil {
		return e
	}
	e := &Edge{ID: r.newID(), FromID: from.ID, ToID: to.ID, Label: label, Kind: edgeKind(from, to)}
	r.p.Edges = append(r.p.Edges, e)
	return e
}

func (r *reducer) removeEdge(id string) {
	out := r.p.Edges[:0]
	for _, e := range r.p.Edges {
		if e.ID != id {
			out = append(out, e)
		}
	}
	r.p.Edges = out
}

func (r *reducer) dropEdgeIfUnowned(from, to, label string) {
	if r.hasOwner(from, to, label) {
		return
	}
	if e := r.findEdge(from, to, label); e != nil {
		r.removeEdge(e.ID)
	}
}

func channelName(ch string) string { return ch }

// errSameChannel is the §6.1 same-channel refusal.
func errSameChannel(from, to *Node) *ActionError {
	return &ActionError{Status: 409, Invariant: "same-channel",
		Message: fmt.Sprintf("edges must connect nodes in the same channel (%s is %s, %s is %s)", from.ID, channelName(from.Channel()), to.ID, channelName(to.Channel())),
		Next:    fmt.Sprintf("call get_canvas and pick a %s node as toId", from.Channel())}
}

// setLinkTarget writes a link target and keeps its edge in sync (§6.1).
func (r *reducer) setLinkTarget(n *Node, l linkRef, to string) error {
	if to != "" {
		tn := r.p.Node(to)
		if tn == nil {
			return errUnknown("node", to)
		}
		if tn.Channel() != n.Channel() {
			return errSameChannel(n, tn)
		}
	}
	old := string(*l.toID)
	if old == to {
		return nil
	}
	*l.toID = Ref(to)
	if old != "" && old != n.ID {
		r.dropEdgeIfUnowned(n.ID, old, *l.label)
	}
	if to != "" && to != n.ID {
		r.ensureEdge(n, r.p.Node(to), *l.label)
	}
	return nil
}

// relabelLink renames a link and keeps its edge label in sync.
func (r *reducer) relabelLink(n *Node, l linkRef, label string) {
	old := *l.label
	if old == label {
		return
	}
	*l.label = label
	to := string(*l.toID)
	if to == "" || to == n.ID {
		return
	}
	oldEdge := r.findEdge(n.ID, to, old)
	stillOwned := r.hasOwner(n.ID, to, old)
	existing := r.findEdge(n.ID, to, label)
	if oldEdge != nil && !stillOwned && existing == nil {
		oldEdge.Label = label
		return
	}
	if oldEdge != nil && !stillOwned {
		r.removeEdge(oldEdge.ID)
	}
	if existing == nil {
		if tn := r.p.Node(to); tn != nil {
			r.ensureEdge(n, tn, label)
		}
	}
}

// clearLinks clears every link target of the given refs (dropping their edges).
func (r *reducer) clearLinks(n *Node, refs []linkRef) {
	for _, l := range refs {
		_ = r.setLinkTarget(n, l, "")
	}
}

// deleteNode removes a node with the §6.1 cascade. Returns counts for the summary.
func (r *reducer) deleteNode(id string) (edges, pins int) {
	n := r.p.Node(id)
	if n == nil {
		return
	}
	// clear every toId pointing at the node
	for _, other := range r.p.Nodes {
		if other.ID == id {
			continue
		}
		for _, l := range nodeLinks(other) {
			if string(*l.toID) == id {
				*l.toID = ""
			}
		}
	}
	keptE := r.p.Edges[:0]
	for _, e := range r.p.Edges {
		if e.FromID == id || e.ToID == id {
			edges++
			continue
		}
		keptE = append(keptE, e)
	}
	r.p.Edges = keptE
	keptP := r.p.Pins[:0]
	for _, p := range r.p.Pins {
		if (p.Target.Type == "node" || p.Target.Type == "board") && p.Target.NodeID == id {
			pins++
			continue
		}
		keptP = append(keptP, p)
	}
	r.p.Pins = keptP
	keptN := r.p.Nodes[:0]
	for _, x := range r.p.Nodes {
		if x.ID != id {
			keptN = append(keptN, x)
		}
	}
	r.p.Nodes = keptN
	if f := r.p.Flow(n.FlowID); f != nil && string(f.StartID) == id {
		f.StartID = ""
		if rest := r.p.NodesOfFlow(f.ID); len(rest) > 0 {
			f.StartID = Ref(rest[0].ID)
		}
	}
	return
}

// recomputeKinds re-derives cross/same-flow kinds for edges touching a node.
func (r *reducer) recomputeKinds(nodeID string) {
	for _, e := range r.p.Edges {
		if e.FromID != nodeID && e.ToID != nodeID {
			continue
		}
		from, to := r.p.Node(e.FromID), r.p.Node(e.ToID)
		if from == nil || to == nil {
			continue
		}
		if from.FlowID != to.FlowID {
			e.Kind = "cross"
		} else if e.Kind == "cross" {
			e.Kind = "primary"
		}
	}
}

// CheckInvariants returns every §6.1 violation in p (empty when valid).
func CheckInvariants(p *Project) []string {
	var errs []string
	add := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	nodes := map[string]*Node{}
	for _, n := range p.Nodes {
		nodes[n.ID] = n
		if p.Flow(n.FlowID) == nil {
			add("node %s references missing flow %s", n.ID, n.FlowID)
		} else if f := p.Flow(n.FlowID); f.Channel != n.Channel() {
			add("node %s kind %s is in %s flow %s", n.ID, n.Kind, f.Channel, f.ID)
		}
	}
	triples := map[string]bool{}
	for _, e := range p.Edges {
		from, to := nodes[e.FromID], nodes[e.ToID]
		if from == nil || to == nil {
			add("edge %s references a missing node", e.ID)
			continue
		}
		if from.Channel() != to.Channel() {
			add("edge %s crosses channels", e.ID)
		}
		if e.FromID == e.ToID {
			add("edge %s is a self edge", e.ID)
		}
		// SPEC: §7.1 lists ed_w11 (Confirm → Cart, both in Checkout) as cross and §17 row 6
		// counts 9 vermillion seed curves. The table wins over the §6.1 prose, so a stored
		// same-flow "cross" is tolerated; the reducer recomputes kind on every create/retarget,
		// and a cross-flow edge must always be "cross".
		cross := from.FlowID != to.FlowID
		if cross && e.Kind != "cross" {
			add("edge %s kind %s but crosses flows", e.ID, e.Kind)
		}
		k := e.FromID + "\x00" + e.ToID + "\x00" + e.Label
		if triples[k] {
			add("edge %s duplicates (%s,%s,%q)", e.ID, e.FromID, e.ToID, e.Label)
		}
		triples[k] = true
	}
	for _, n := range p.Nodes {
		for _, l := range nodeLinks(n) {
			to := string(*l.toID)
			if to == "" {
				continue
			}
			tn := nodes[to]
			if tn == nil {
				add("link %s on %s targets missing node %s", l.key, n.ID, to)
				continue
			}
			if to == n.ID {
				continue
			}
			if !triples[n.ID+"\x00"+to+"\x00"+*l.label] {
				add("link %s on %s → %s (%q) has no edge", l.key, n.ID, to, *l.label)
			}
		}
	}
	chFlows := map[string]int{}
	for _, f := range p.Flows {
		chFlows[f.Channel]++
		if f.StartID != "" {
			sn := nodes[string(f.StartID)]
			if sn == nil || sn.FlowID != f.ID {
				add("flow %s startId %s is not in the flow", f.ID, f.StartID)
			}
		}
	}
	if chFlows[ChanWeb] < 1 || chFlows[ChanMobile] < 1 {
		add("each channel must keep at least one flow")
	}
	maxN := 0
	for _, x := range p.Pins {
		if x.N > maxN {
			maxN = x.N
		}
		if (x.Target.Type == "node" || x.Target.Type == "board") && nodes[x.Target.NodeID] == nil {
			add("pin %s targets missing node %s", x.ID, x.Target.NodeID)
		}
	}
	if maxN > p.PinCounter {
		add("pinCounter %d below pin number %d", p.PinCounter, maxN)
	}
	if len(p.Tokens.Space) != 8 {
		add("tokens.space must have 8 entries")
	} else {
		for i, v := range SpaceScale {
			if p.Tokens.Space[i] != v {
				add("tokens.space differs from the fixed scale")
				break
			}
		}
	}
	return errs
}
