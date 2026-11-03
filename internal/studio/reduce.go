package studio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Action is a named intent applied by the reducer (§12.3).
type Action struct {
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	Tab      string          `json:"tab,omitempty"`
	Coalesce string          `json:"coalesce,omitempty"`
	Commit   bool            `json:"commit,omitempty"`
}

// ActionError is a refusal: the project is unchanged.
type ActionError struct {
	Status    int    `json:"-"`
	Invariant string `json:"invariant,omitempty"`
	Message   string `json:"message"`
	Next      string `json:"next,omitempty"`
}

func (e *ActionError) Error() string {
	s := "Refused: " + e.Message + "."
	if e.Next != "" {
		s += " Next: " + e.Next + "."
	}
	return s
}

func errUnknown(kind, id string) *ActionError {
	return &ActionError{Status: 404, Invariant: "unknown-id",
		Message: fmt.Sprintf("unknown %s id %q", kind, id),
		Next:    "call get_canvas to list valid ids"}
}

func errInvalid(msg, next string) *ActionError {
	return &ActionError{Status: 400, Invariant: "invalid-payload", Message: msg, Next: next}
}

func errInv(inv, msg, next string) *ActionError {
	return &ActionError{Status: 409, Invariant: inv, Message: msg, Next: next}
}

// Outcome describes a successfully reduced action.
type Outcome struct {
	History   string // "", "web", "story", "draw", "doc"
	Summary   string
	CreatedID string
}

type reducer struct {
	p    *Project
	v    *SharedView
	a    Action
	now  time.Time
	ids  map[string]bool
	out  Outcome
	tabs map[string]*Selection
}

// Reduce applies a to p and v in place. p and v must be private copies:
// on error they may be partially modified and must be discarded.
func Reduce(p *Project, v *SharedView, tabSel map[string]*Selection, a Action, now time.Time) (Outcome, error) {
	r := &reducer{p: p, v: v, a: a, now: now, tabs: tabSel}
	h, ok := handlers[a.Type]
	if !ok {
		return Outcome{}, errInvalid(fmt.Sprintf("unknown action type %q", a.Type), "use one of: "+strings.Join(ActionTypes(), ", "))
	}
	if err := h(r); err != nil {
		return Outcome{}, err
	}
	fixView(p, v, tabSel)
	return r.out, nil
}

// ActionTypes lists every reducer case (sorted).
func ActionTypes() []string {
	out := make([]string, 0, len(handlers)+3)
	for k := range handlers {
		out = append(out, k)
	}
	out = append(out, "undo", "redo", "reset_project")
	sort.Strings(out)
	return out
}

var handlers map[string]func(*reducer) error

func init() {
	handlers = map[string]func(*reducer) error{
		"set_tab":          (*reducer).setTab,
		"set_view":         (*reducer).setView,
		"set_breakpoint":   (*reducer).setBreakpoint,
		"set_current_node": (*reducer).setCurrentNode,
		"set_selection":    (*reducer).setSelection,
		"set_toggles":      (*reducer).setToggles,
		"set_preview":      (*reducer).setPreview,
		"set_draw_view":    (*reducer).setDrawView,
		"update_tokens":    (*reducer).updateTokens,
		"upsert_flow":      (*reducer).upsertFlow,
		"delete_flow":      (*reducer).deleteFlowA,
		"upsert_node":      (*reducer).upsertNode,
		"reorder_nodes":    (*reducer).reorderNodes,
		"delete_node":      (*reducer).deleteNodeA,
		"insert_block":     (*reducer).insertBlock,
		"update_block":     (*reducer).updateBlock,
		"upsert_child":     (*reducer).upsertChild,
		"delete_child":     (*reducer).deleteChild,
		"reorder_blocks":   (*reducer).reorderBlocks,
		"duplicate_block":  (*reducer).duplicateBlock,
		"delete_block":     (*reducer).deleteBlock,
		"update_screen":    (*reducer).updateScreen,
		"upsert_hotspot":   (*reducer).upsertHotspot,
		"delete_hotspot":   (*reducer).deleteHotspot,
		"set_link_target":  (*reducer).setLinkTargetA,
		"upsert_edge":      (*reducer).upsertEdge,
		"delete_edge":      (*reducer).deleteEdgeA,
		"add_shape":        (*reducer).addShape,
		"update_shape":     (*reducer).updateShape,
		"duplicate_shape":  (*reducer).duplicateShape,
		"delete_shape":     (*reducer).deleteShape,
		"set_doc":          (*reducer).setDoc,
		"add_pin":          (*reducer).addPin,
		"update_pin":       (*reducer).updatePin,
		"delete_pin":       (*reducer).deletePin,
	}
}

// ---- helpers ----

func (r *reducer) decode(dst any) error {
	raw := r.a.Payload
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errInvalid(fmt.Sprintf("bad payload for %s: %v", r.a.Type, err), "check the payload fields in the tool input schema")
	}
	return nil
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// newID returns a fresh 8-char base36 id unique within the project.
func (r *reducer) newID() string {
	if r.ids == nil {
		r.ids = collectIDs(r.p)
	}
	for {
		b := make([]byte, 8)
		for i := range b {
			b[i] = idAlphabet[rand.IntN(len(idAlphabet))]
		}
		id := string(b)
		if !r.ids[id] {
			r.ids[id] = true
			return id
		}
	}
}

func collectIDs(p *Project) map[string]bool {
	ids := map[string]bool{}
	for _, f := range p.Flows {
		ids[f.ID] = true
	}
	for _, n := range p.Nodes {
		ids[n.ID] = true
		for _, b := range n.Blocks {
			ids[b.ID] = true
			for _, l := range b.Links {
				ids[l.ID] = true
			}
			for _, it := range b.Items {
				ids[it.ID] = true
			}
			for _, l := range []*Link{b.CTA, b.Secondary, b.Button} {
				if l != nil {
					ids[l.ID] = true
				}
			}
		}
		for _, h := range n.Hotspots {
			ids[h.ID] = true
		}
	}
	for _, e := range p.Edges {
		ids[e.ID] = true
	}
	for _, s := range p.Draw.Shapes {
		ids[s.ID] = true
	}
	for _, x := range p.Pins {
		ids[x.ID] = true
	}
	return ids
}

func histFor(ch string) string {
	if ch == ChanMobile {
		return "story"
	}
	return "web"
}

func tabFor(ch string) string { return histFor(ch) }

func (r *reducer) node(id string) (*Node, error) {
	if id == "" {
		return nil, errInvalid("nodeId is required", "call get_canvas to list node ids")
	}
	n := r.p.Node(id)
	if n == nil {
		return nil, errUnknown("node", id)
	}
	return n, nil
}

func (r *reducer) page(id string) (*Node, error) {
	n, err := r.node(id)
	if err != nil {
		return nil, err
	}
	if n.Kind != "page" {
		return nil, errInv("node-kind", fmt.Sprintf("%s is a mobile screen, not a web page", id), "use update_screen / upsert_hotspot for screens")
	}
	return n, nil
}

func (r *reducer) screen(id string) (*Node, error) {
	n, err := r.node(id)
	if err != nil {
		return nil, err
	}
	if n.Kind != "screen" {
		return nil, errInv("node-kind", fmt.Sprintf("%s is a web page, not a mobile screen", id), "use update_block / insert_block for pages")
	}
	return n, nil
}

func (r *reducer) block(n *Node, id string) (*Block, error) {
	if id == "" {
		return nil, errInvalid("blockId is required", "call get_node to list block ids")
	}
	b := n.Block(id)
	if b == nil {
		return nil, errUnknown("block", id)
	}
	return b, nil
}

func checkEnum(field, v string, allowed []string) error {
	if !contains(allowed, v) {
		return errInvalid(fmt.Sprintf("%s must be one of %s (got %q)", field, strings.Join(allowed, ", "), v), "send a valid value")
	}
	return nil
}

func checkSpace(field string, v int) error {
	if v < 0 || v > 7 {
		return errInvalid(fmt.Sprintf("%s is a space index 0–7 into [0 4 8 16 24 32 48 64] (got %d)", field, v), "send an index, not a px value")
	}
	return nil
}

func checkWeight(field string, v int) error {
	if v != 400 && v != 500 && v != 600 {
		return errInvalid(fmt.Sprintf("%s must be 400, 500, or 600 (got %d)", field, v), "send a valid weight")
	}
	return nil
}

var hexRe = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

func normHex(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !hexRe.MatchString(s) {
		return "", false
	}
	h := strings.TrimPrefix(s, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	return "#" + strings.ToUpper(h), true
}

func quote(s string) string {
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:57]) + "…"
	}
	return fmt.Sprintf("%q", s)
}

func (r *reducer) selectItem(s *Selection) {
	r.v.Selection = s
}

// ---- view actions ----

func (r *reducer) switchTab(tab string) {
	if r.v.Tab == tab {
		return
	}
	if r.tabs != nil {
		r.tabs[r.v.Tab] = r.v.Selection
		r.v.Selection = r.tabs[tab]
	} else {
		r.v.Selection = nil
	}
	r.v.Tab = tab
}

func (r *reducer) setTab() error {
	var pl struct {
		Tab  string `json:"tab"`
		View string `json:"view,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if err := checkEnum("tab", pl.Tab, Tabs); err != nil {
		return err
	}
	r.switchTab(pl.Tab)
	if pl.View != "" {
		if err := r.applyView(pl.Tab, pl.View); err != nil {
			return err
		}
	}
	r.out.Summary = "Switched to the " + tabTitle(pl.Tab) + " tab"
	if pl.View != "" {
		r.out.Summary += " (" + viewTitle(pl.View) + ")"
	}
	return nil
}

func tabTitle(t string) string {
	switch t {
	case "web":
		return "Web"
	case "story":
		return "Story"
	case "draw":
		return "Draw"
	case "doc":
		return "Doc"
	}
	return t
}

func viewTitle(v string) string {
	if v == "board" {
		return "Storyboard"
	}
	return "Page"
}

func (r *reducer) applyView(tab, view string) error {
	if err := checkEnum("view", view, []string{"page", "board"}); err != nil {
		return err
	}
	switch tab {
	case "web":
		r.v.View.Web = view
	case "story":
		r.v.View.Story = view
	default:
		return errInvalid("view applies only to the web and story tabs", "omit view for draw and doc")
	}
	return nil
}

func (r *reducer) setView() error {
	var pl struct {
		Tab  string `json:"tab"`
		View string `json:"view"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if err := r.applyView(pl.Tab, pl.View); err != nil {
		return err
	}
	r.out.Summary = "Showing " + viewTitle(pl.View) + " view in " + tabTitle(pl.Tab)
	return nil
}

func (r *reducer) setBreakpoint() error {
	var pl struct {
		Breakpoint string `json:"breakpoint"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if err := checkEnum("breakpoint", pl.Breakpoint, []string{"desktop", "tablet", "phone"}); err != nil {
		return err
	}
	r.v.Breakpoint = pl.Breakpoint
	r.out.Summary = fmt.Sprintf("Set breakpoint to %s (%dpx)", pl.Breakpoint, Breakpoints[pl.Breakpoint])
	return nil
}

func (r *reducer) makeCurrent(n *Node) {
	if n.Channel() == ChanWeb {
		r.v.CurrentNodeID.Web = Ref(n.ID)
	} else {
		r.v.CurrentNodeID.Mobile = Ref(n.ID)
	}
	r.switchTab(tabFor(n.Channel()))
}

func (r *reducer) setCurrentNode() error {
	var pl struct {
		NodeID string `json:"nodeId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.node(pl.NodeID)
	if err != nil {
		return err
	}
	r.makeCurrent(n)
	if s := r.v.Selection; s != nil && s.NodeID != "" && s.NodeID != n.ID {
		r.v.Selection = nil
	}
	r.out.Summary = "Opened " + n.Name
	return nil
}

func (r *reducer) validSelection(s *Selection) error {
	switch s.Kind {
	case "node":
		_, err := r.node(s.NodeID)
		return err
	case "block":
		n, err := r.page(s.NodeID)
		if err != nil {
			return err
		}
		b, err := r.block(n, s.BlockID)
		if err != nil {
			return err
		}
		if s.ChildID != "" && !blockHasChild(b, s.ChildID) {
			return errUnknown("child", s.ChildID)
		}
	case "hotspot":
		n, err := r.screen(s.NodeID)
		if err != nil {
			return err
		}
		if n.Hotspot(s.HotspotID) == nil {
			return errUnknown("hotspot", s.HotspotID)
		}
	case "edge":
		if r.p.Edge(s.EdgeID) == nil {
			return errUnknown("edge", s.EdgeID)
		}
	case "shape":
		if r.p.Shape(s.ShapeID) == nil {
			return errUnknown("shape", s.ShapeID)
		}
	case "pin":
		if r.p.Pin(s.PinID) == nil {
			return errUnknown("pin", s.PinID)
		}
	default:
		return errInvalid(fmt.Sprintf("selection kind must be node, block, hotspot, edge, shape, or pin (got %q)", s.Kind), "send a valid selection or null")
	}
	return nil
}

func blockHasChild(b *Block, id string) bool {
	for _, l := range b.Links {
		if l.ID == id {
			return true
		}
	}
	for _, it := range b.Items {
		if it.ID == id {
			return true
		}
	}
	for _, l := range []*Link{b.CTA, b.Secondary, b.Button} {
		if l != nil && l.ID == id {
			return true
		}
	}
	return false
}

func (r *reducer) setSelection() error {
	var pl struct {
		Selection *Selection `json:"selection"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	s := pl.Selection
	if s == nil {
		r.v.Selection = nil
		r.out.Summary = "Cleared the selection"
		return nil
	}
	if err := r.validSelection(s); err != nil {
		return err
	}
	switch s.Kind {
	case "node", "block", "hotspot":
		n := r.p.Node(s.NodeID)
		r.makeCurrent(n)
		r.out.Summary = "Selected " + r.selectionLabel(s)
	case "edge":
		e := r.p.Edge(s.EdgeID)
		if from := r.p.Node(e.FromID); from != nil {
			r.switchTab(tabFor(from.Channel()))
		}
		r.out.Summary = "Selected " + r.selectionLabel(s)
	case "shape":
		r.switchTab("draw")
		r.out.Summary = "Selected " + r.selectionLabel(s)
	case "pin":
		// SPEC: selecting a pin also switches tab/view/node to show it (§10 Review jump).
		x := r.p.Pin(s.PinID)
		switch x.Target.Type {
		case "draw":
			r.switchTab("draw")
		default:
			if n := r.p.Node(x.Target.NodeID); n != nil {
				r.makeCurrent(n)
				view := "page"
				if x.Target.Type == "board" {
					view = "board"
				}
				_ = r.applyView(tabFor(n.Channel()), view)
			}
		}
		r.out.Summary = fmt.Sprintf("Selected pin %d", x.N)
	}
	r.v.Selection = s
	return nil
}

func (r *reducer) selectionLabel(s *Selection) string {
	switch s.Kind {
	case "node":
		if n := r.p.Node(s.NodeID); n != nil {
			return n.Name
		}
	case "block":
		n := r.p.Node(s.NodeID)
		b := n.Block(s.BlockID)
		l := n.Name + " › " + BlockLabel(b.Type)
		if s.ChildID != "" {
			l += " › " + childLabel(b, s.ChildID)
		}
		return l
	case "hotspot":
		n := r.p.Node(s.NodeID)
		return n.Name + " › " + n.Hotspot(s.HotspotID).Label
	case "edge":
		e := r.p.Edge(s.EdgeID)
		return fmt.Sprintf("edge %s → %s %s", r.nodeName(e.FromID), r.nodeName(e.ToID), quote(e.Label))
	case "shape":
		return r.p.Shape(s.ShapeID).Type
	}
	return s.Kind
}

func childLabel(b *Block, id string) string {
	for _, l := range b.Links {
		if l.ID == id {
			return l.Label
		}
	}
	for _, it := range b.Items {
		if it.ID == id {
			return it.Title
		}
	}
	for _, l := range []*Link{b.CTA, b.Secondary, b.Button} {
		if l != nil && l.ID == id {
			return l.Label
		}
	}
	return id
}

func (r *reducer) nodeName(id string) string {
	if n := r.p.Node(id); n != nil {
		return n.Name
	}
	return id
}

func (r *reducer) setToggles() error {
	var pl struct {
		ShowPins  *bool `json:"showPins,omitempty"`
		ShowGrid  *bool `json:"showGrid,omitempty"`
		ShowSpecs *bool `json:"showSpecs,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	var parts []string
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	if pl.ShowPins != nil {
		r.v.ShowPins = *pl.ShowPins
		parts = append(parts, "pins "+onOff(*pl.ShowPins))
	}
	if pl.ShowGrid != nil {
		r.v.ShowGrid = *pl.ShowGrid
		parts = append(parts, "grid "+onOff(*pl.ShowGrid))
	}
	if pl.ShowSpecs != nil {
		r.v.ShowSpecs = *pl.ShowSpecs
		parts = append(parts, "specs "+onOff(*pl.ShowSpecs))
	}
	if len(parts) == 0 {
		return errInvalid("set at least one of showPins, showGrid, showSpecs", "send a boolean toggle")
	}
	r.out.Summary = "Turned " + strings.Join(parts, ", ")
	return nil
}

func (r *reducer) setPreview() error {
	var pl struct {
		On bool `json:"on"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	r.v.Preview = pl.On
	if pl.On {
		r.out.Summary = "Entered Preview"
	} else {
		r.out.Summary = "Exited Preview"
	}
	return nil
}

func (r *reducer) setDrawView() error {
	var pl struct {
		X    float64 `json:"x"`
		Y    float64 `json:"y"`
		Zoom float64 `json:"zoom"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	z := pl.Zoom
	if z == 0 {
		z = 1
	}
	z = math.Max(0.25, math.Min(4, z))
	r.p.Draw.View = DrawView{X: pl.X, Y: pl.Y, Zoom: z}
	r.out.Summary = fmt.Sprintf("Draw view at %d%%", int(math.Round(z*100)))
	return nil
}

// ---- tokens ----

func (r *reducer) updateTokens() error {
	var pl struct {
		Colors map[string]string `json:"colors,omitempty"`
		Type   map[string]struct {
			Size   *int `json:"size,omitempty"`
			LH     *int `json:"lh,omitempty"`
			Weight *int `json:"weight,omitempty"`
		} `json:"type,omitempty"`
		Space json.RawMessage `json:"space,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if pl.Space != nil {
		return errInv("fixed-space", "tokens.space is the fixed 8-step scale [0 4 8 16 24 32 48 64] and cannot be edited", "change pad/gap indices on blocks or screens instead")
	}
	var parts []string
	keys := make([]string, 0, len(pl.Colors))
	for k := range pl.Colors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := checkEnum("color key", k, ColorKeys); err != nil {
			return err
		}
		h, ok := normHex(pl.Colors[k])
		if !ok {
			return errInvalid(fmt.Sprintf("color %s must be a hex like #D8432A (got %q)", k, pl.Colors[k]), "send a 6-digit hex")
		}
		r.p.Tokens.Colors[k] = h
		parts = append(parts, k+" "+h)
	}
	tkeys := make([]string, 0, len(pl.Type))
	for k := range pl.Type {
		tkeys = append(tkeys, k)
	}
	sort.Strings(tkeys)
	for _, k := range tkeys {
		if err := checkEnum("type key", k, TypeKeys); err != nil {
			return err
		}
		t := pl.Type[k]
		spec := r.p.Tokens.Type[k]
		if t.Size != nil {
			if *t.Size < 6 || *t.Size > 200 {
				return errInvalid(fmt.Sprintf("type %s size must be 6–200 px (got %d)", k, *t.Size), "send a px size")
			}
			spec.Size = *t.Size
		}
		if t.LH != nil {
			if *t.LH < 6 || *t.LH > 300 {
				return errInvalid(fmt.Sprintf("type %s line-height must be 6–300 px (got %d)", k, *t.LH), "send a px line-height")
			}
			spec.LH = *t.LH
		}
		if t.Weight != nil {
			if err := checkWeight("type "+k+" weight", *t.Weight); err != nil {
				return err
			}
			spec.Weight = *t.Weight
		}
		r.p.Tokens.Type[k] = spec
		parts = append(parts, fmt.Sprintf("%s %d/%d", k, spec.Size, spec.LH))
	}
	if len(parts) == 0 {
		return errInvalid("send colors and/or type to change", "e.g. {\"colors\":{\"accent\":\"#0055FF\"}}")
	}
	r.out.Summary = "Set tokens: " + strings.Join(parts, ", ")
	// SPEC: token edits sent from the draw or doc tab (no token history there) use the web history.
	r.out.History = "web"
	if r.a.Tab == "story" {
		r.out.History = "story"
	}
	return nil
}

// ---- flows and nodes ----

var archetypes = []string{"title", "photo", "quote", "cta", "split"}

// NewScreen returns a screen with the §8.4 archetype defaults.
func newScreen(id, flowID, name, archetype string) *Node {
	n := &Node{Kind: "screen", ID: id, FlowID: flowID, Name: name, Archetype: archetype,
		Fill: "paper", Ink: "ink", Weight: 500, Pad: 4, Gap: 3, Align: "left", TypeRole: "xl",
		Hotspots: []*Hotspot{},
		CTA:      &ScreenCTA{Label: "Continue", Size: "md", Fill: "accent", Ink: "paper"}}
	switch archetype {
	case "title":
		n.Layout, n.Align, n.TypeRole, n.Weight = "center", "center", "display", 600
		n.Title, n.Body = "A clear title", "One short line that sets up the screen."
	case "photo":
		n.Layout, n.Fill, n.Ink = "bottom", "ink", "paper"
		n.Title, n.Body = "A photo-led moment", "A caption that earns the image."
	case "quote":
		n.Layout, n.Align, n.TypeRole = "center", "center", "lg"
		n.Title, n.Body = "“A line worth quoting.”", "Name, place"
		n.CTA = nil
	case "cta":
		n.Layout = "bottom"
		n.Title, n.Body = "One clear action", "Say what happens when they tap."
	case "split":
		n.Layout = "split"
		n.Title, n.Body = "Two halves", "Title above, detail and action below."
	}
	return n
}

func (r *reducer) upsertFlow() error {
	var pl struct {
		ID      string          `json:"id,omitempty"`
		Channel string          `json:"channel,omitempty"`
		Name    *string         `json:"name,omitempty"`
		StartID json.RawMessage `json:"startId,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if pl.ID == "" {
		if err := checkEnum("channel", pl.Channel, []string{ChanWeb, ChanMobile}); err != nil {
			return err
		}
		name := "Untitled flow"
		if pl.Name != nil && strings.TrimSpace(*pl.Name) != "" {
			name = strings.TrimSpace(*pl.Name)
		}
		f := &Flow{ID: r.newID(), Name: name, Channel: pl.Channel}
		r.p.Flows = append(r.p.Flows, f)
		var n *Node
		if pl.Channel == ChanWeb {
			n = &Node{Kind: "page", ID: r.newID(), FlowID: f.ID, Name: "Untitled page", Blocks: []*Block{}}
		} else {
			n = newScreen(r.newID(), f.ID, "Untitled screen", "title")
		}
		r.p.Nodes = append(r.p.Nodes, n)
		f.StartID = Ref(n.ID)
		r.makeCurrent(n)
		r.selectItem(&Selection{Kind: "node", NodeID: n.ID})
		r.out.CreatedID = f.ID
		r.out.History = histFor(pl.Channel)
		r.out.Summary = fmt.Sprintf("Created %s flow %s with %s", pl.Channel, quote(name), quote(n.Name))
		return nil
	}
	f := r.p.Flow(pl.ID)
	if f == nil {
		return errUnknown("flow", pl.ID)
	}
	if pl.Channel != "" && pl.Channel != f.Channel {
		return errInv("flow-channel", fmt.Sprintf("flow %s is %s; a flow cannot change channel", f.ID, f.Channel), "create a new flow in the other channel")
	}
	var parts []string
	if pl.Name != nil {
		name := strings.TrimSpace(*pl.Name)
		if name == "" {
			return errInvalid("flow name cannot be empty", "send a non-empty name")
		}
		parts = append(parts, fmt.Sprintf("renamed %s to %s", quote(f.Name), quote(name)))
		f.Name = name
	}
	if pl.StartID != nil {
		var sid Ref
		if err := json.Unmarshal(pl.StartID, &sid); err != nil {
			return errInvalid("startId must be a node id or null", "send a node id of this flow")
		}
		if sid == "" {
			if len(r.p.NodesOfFlow(f.ID)) > 0 {
				return errInv("start-in-flow", fmt.Sprintf("flow %s has nodes, so its startId must be one of them", f.ID), "send a node id of this flow")
			}
		} else {
			sn := r.p.Node(string(sid))
			if sn == nil {
				return errUnknown("node", string(sid))
			}
			if sn.FlowID != f.ID {
				return errInv("start-in-flow", fmt.Sprintf("flow.startId must belong to that flow (%s is in %s, not %s)", sn.ID, sn.FlowID, f.ID), "pick a node of "+f.ID+" or move the node first")
			}
			parts = append(parts, "start is "+sn.Name)
		}
		f.StartID = sid
	}
	if len(parts) == 0 {
		return errInvalid("nothing to change: send name or startId", "include a field to update")
	}
	r.out.History = histFor(f.Channel)
	r.out.Summary = "Flow " + f.Name + ": " + strings.Join(parts, ", ")
	return nil
}

func (r *reducer) deleteFlowA() error {
	var pl struct {
		FlowID string `json:"flowId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	f := r.p.Flow(pl.FlowID)
	if f == nil {
		return errUnknown("flow", pl.FlowID)
	}
	if len(r.p.FlowsOf(f.Channel)) <= 1 {
		return errInv("last-flow", fmt.Sprintf("a channel must keep at least one flow (%s is the last %s flow)", f.Name, f.Channel), "create another flow with upsert_flow first, or rename this one")
	}
	nodes := r.p.NodesOfFlow(f.ID)
	for _, n := range nodes {
		r.deleteNode(n.ID)
	}
	kept := r.p.Flows[:0]
	for _, x := range r.p.Flows {
		if x.ID != f.ID {
			kept = append(kept, x)
		}
	}
	r.p.Flows = kept
	r.out.History = histFor(f.Channel)
	r.out.Summary = fmt.Sprintf("Deleted flow %s and its %d node(s)", quote(f.Name), len(nodes))
	return nil
}

func (r *reducer) upsertNode() error {
	var pl struct {
		ID        string  `json:"id,omitempty"`
		FlowID    string  `json:"flowId,omitempty"`
		Name      *string `json:"name,omitempty"`
		Archetype string  `json:"archetype,omitempty"`
		AfterID   string  `json:"afterId,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if pl.ID == "" {
		f := r.p.Flow(pl.FlowID)
		if f == nil {
			if pl.FlowID == "" {
				return errInvalid("flowId is required to create a node", "call get_canvas to list flow ids")
			}
			return errUnknown("flow", pl.FlowID)
		}
		var n *Node
		if f.Channel == ChanWeb {
			if pl.Archetype != "" {
				return errInvalid("archetype applies only to mobile screens", "omit archetype for web pages")
			}
			name := "Untitled page"
			if pl.Name != nil && strings.TrimSpace(*pl.Name) != "" {
				name = strings.TrimSpace(*pl.Name)
			}
			n = &Node{Kind: "page", ID: r.newID(), FlowID: f.ID, Name: name, Blocks: []*Block{}}
		} else {
			arch := pl.Archetype
			if arch == "" {
				arch = "title"
			}
			if err := checkEnum("archetype", arch, archetypes); err != nil {
				return err
			}
			name := "Untitled screen"
			if pl.Name != nil && strings.TrimSpace(*pl.Name) != "" {
				name = strings.TrimSpace(*pl.Name)
			}
			n = newScreen(r.newID(), f.ID, name, arch)
		}
		// insert after afterId, else after the flow's last node, else at the end
		idx := -1
		if pl.AfterID != "" {
			for i, x := range r.p.Nodes {
				if x.ID == pl.AfterID {
					idx = i
				}
			}
			if idx < 0 {
				return errUnknown("node", pl.AfterID)
			}
		} else {
			for i, x := range r.p.Nodes {
				if x.FlowID == f.ID {
					idx = i
				}
			}
		}
		if idx < 0 {
			r.p.Nodes = append(r.p.Nodes, n)
		} else {
			r.p.Nodes = append(r.p.Nodes[:idx+1], append([]*Node{n}, r.p.Nodes[idx+1:]...)...)
		}
		if f.StartID == "" {
			f.StartID = Ref(n.ID)
		}
		r.makeCurrent(n)
		r.selectItem(&Selection{Kind: "node", NodeID: n.ID})
		r.out.CreatedID = n.ID
		r.out.History = histFor(f.Channel)
		r.out.Summary = fmt.Sprintf("Added %s %s to %s", n.Kind, quote(n.Name), f.Name)
		return nil
	}
	n, err := r.node(pl.ID)
	if err != nil {
		return err
	}
	var parts []string
	if pl.Name != nil {
		name := strings.TrimSpace(*pl.Name)
		if name == "" {
			return errInvalid("node name cannot be empty", "send a non-empty name")
		}
		parts = append(parts, fmt.Sprintf("renamed %s to %s", quote(n.Name), quote(name)))
		n.Name = name
	}
	if pl.Archetype != "" {
		if n.Kind != "screen" {
			return errInvalid("archetype applies only to mobile screens", "omit archetype for web pages")
		}
		if err := checkEnum("archetype", pl.Archetype, archetypes); err != nil {
			return err
		}
		n.Archetype = pl.Archetype
		parts = append(parts, "archetype "+pl.Archetype)
	}
	if pl.FlowID != "" && pl.FlowID != n.FlowID {
		f := r.p.Flow(pl.FlowID)
		if f == nil {
			return errUnknown("flow", pl.FlowID)
		}
		if f.Channel != n.Channel() {
			return errInv("same-channel", fmt.Sprintf("a %s cannot move into %s flow %s", n.Kind, f.Channel, f.ID), "pick a flow of the same channel")
		}
		oldFlow := r.p.Flow(n.FlowID)
		n.FlowID = f.ID
		if oldFlow != nil && string(oldFlow.StartID) == n.ID {
			oldFlow.StartID = ""
			if rest := r.p.NodesOfFlow(oldFlow.ID); len(rest) > 0 {
				oldFlow.StartID = Ref(rest[0].ID)
			}
		}
		if f.StartID == "" {
			f.StartID = Ref(n.ID)
		}
		r.recomputeKinds(n.ID)
		parts = append(parts, "moved to "+f.Name)
	}
	if pl.AfterID != "" {
		return errInvalid("afterId applies only when creating", "use reorder_nodes to reorder")
	}
	if len(parts) == 0 {
		return errInvalid("nothing to change: send name, archetype, or flowId", "include a field to update")
	}
	r.out.History = histFor(n.Channel())
	r.out.Summary = n.Name + ": " + strings.Join(parts, ", ")
	return nil
}

func samePermutation(have, want []string) bool {
	if len(have) != len(want) {
		return false
	}
	a := append([]string(nil), have...)
	b := append([]string(nil), want...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *reducer) reorderNodes() error {
	var pl struct {
		FlowID  string   `json:"flowId"`
		NodeIDs []string `json:"nodeIds"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	f := r.p.Flow(pl.FlowID)
	if f == nil {
		return errUnknown("flow", pl.FlowID)
	}
	var have []string
	var slots []int
	for i, n := range r.p.Nodes {
		if n.FlowID == f.ID {
			have = append(have, n.ID)
			slots = append(slots, i)
		}
	}
	if !samePermutation(have, pl.NodeIDs) {
		return errInvalid(fmt.Sprintf("nodeIds must list exactly the nodes of %s: %s", f.ID, strings.Join(have, ", ")), "send every node id of the flow once")
	}
	byID := map[string]*Node{}
	for _, n := range r.p.Nodes {
		byID[n.ID] = n
	}
	for i, id := range pl.NodeIDs {
		r.p.Nodes[slots[i]] = byID[id]
	}
	r.out.History = histFor(f.Channel)
	r.out.Summary = "Reordered " + f.Name
	return nil
}

func (r *reducer) deleteNodeA() error {
	var pl struct {
		NodeID string `json:"nodeId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.node(pl.NodeID)
	if err != nil {
		return err
	}
	ch := n.Channel()
	cur := r.v.CurrentNodeID.Web
	if ch == ChanMobile {
		cur = r.v.CurrentNodeID.Mobile
	}
	next := ""
	if string(cur) == n.ID {
		if f := r.p.Flow(n.FlowID); f != nil {
			lane := LaneOrder(f, r.p.Nodes, r.p.Edges)
			for i, x := range lane {
				if x.ID == n.ID && i+1 < len(lane) {
					next = lane[i+1].ID
				}
			}
		}
	}
	edges, pins := r.deleteNode(n.ID)
	if string(cur) == n.ID {
		if next == "" {
			if f := r.p.Flow(n.FlowID); f != nil && f.StartID != "" {
				next = string(f.StartID)
			}
		}
		if next == "" {
			for _, x := range r.p.Nodes {
				if x.Channel() == ch {
					next = x.ID
					break
				}
			}
		}
		if ch == ChanWeb {
			r.v.CurrentNodeID.Web = Ref(next)
		} else {
			r.v.CurrentNodeID.Mobile = Ref(next)
		}
	}
	r.out.History = histFor(ch)
	r.out.Summary = fmt.Sprintf("Deleted %s %s (%d edge(s), %d pin(s) removed)", n.Kind, quote(n.Name), edges, pins)
	return nil
}

// ---- blocks ----

// NewBlock returns a block with per-type defaults.
func (r *reducer) newBlock(typ string) *Block {
	b := &Block{ID: r.newID(), Type: typ, Fill: "paper", Ink: "ink", Pad: 5, Gap: 3, TypeRole: "lg", Weight: 500, Align: "left"}
	switch typ {
	case "nav":
		b.Pad, b.Gap, b.TypeRole, b.Weight = 3, 4, "sm", 400
		b.Brand = "Brand"
		b.Links = []*Link{{ID: r.newID(), Label: "Shop"}, {ID: r.newID(), Label: "About"}}
	case "hero":
		b.TypeRole, b.Weight = "xl", 500
		b.Eyebrow, b.Headline, b.Body, b.BodyRole = "Eyebrow", "A clear, short headline", "One or two sentences that explain the value.", "md"
		b.CTA = &Link{ID: r.newID(), Label: "Primary action"}
	case "features":
		b.Fill, b.Gap = "wash", 4
		b.Heading = "Features"
		b.Items = []*Item{
			{ID: r.newID(), Title: "Feature one", Body: "One sentence about it."},
			{ID: r.newID(), Title: "Feature two", Body: "One sentence about it."},
			{ID: r.newID(), Title: "Feature three", Body: "One sentence about it."},
		}
	case "quote":
		b.Ink, b.TypeRole, b.Align = "muted", "xl", "center"
		b.Text, b.Cite = "A quote from someone who uses it.", "Name, City"
	case "cta":
		b.Fill = "wash"
		b.Headline, b.Body = "Ready when you are.", "A short supporting line."
		b.Button = &Link{ID: r.newID(), Label: "Get started"}
		b.ButtonFill, b.ButtonInk = "accent", "paper"
	case "footer":
		b.Fill, b.Ink, b.Pad, b.TypeRole, b.Weight = "ink", "paper", 4, "sm", 400
		b.Brand, b.Note = "Brand", "© Studio"
		b.Links = []*Link{{ID: r.newID(), Label: "About"}, {ID: r.newID(), Label: "Contact"}}
	}
	return b
}

func blockIndex(n *Node, id string) int {
	for i, b := range n.Blocks {
		if b.ID == id {
			return i
		}
	}
	return -1
}

func (r *reducer) insertBlock() error {
	var pl struct {
		NodeID       string `json:"nodeId"`
		Type         string `json:"type"`
		AfterBlockID string `json:"afterBlockId,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	if err := checkEnum("type", pl.Type, BlockTypes); err != nil {
		return err
	}
	b := r.newBlock(pl.Type)
	idx := len(n.Blocks)
	if pl.AfterBlockID != "" {
		i := blockIndex(n, pl.AfterBlockID)
		if i < 0 {
			return errUnknown("block", pl.AfterBlockID)
		}
		idx = i + 1
	}
	n.Blocks = append(n.Blocks[:idx], append([]*Block{b}, n.Blocks[idx:]...)...)
	r.makeCurrent(n)
	r.selectItem(&Selection{Kind: "block", NodeID: n.ID, BlockID: b.ID})
	r.out.CreatedID = b.ID
	r.out.History = "web"
	r.out.Summary = fmt.Sprintf("Inserted %s into %s", BlockLabel(b.Type), n.Name)
	return nil
}

// field applicability per block type
var blockFields = map[string][]string{
	"nav":      {"brand"},
	"hero":     {"eyebrow", "headline", "body", "bodyRole", "cta", "secondary"},
	"features": {"heading"},
	"quote":    {"text", "cite"},
	"cta":      {"headline", "body", "button", "buttonFill", "buttonInk"},
	"footer":   {"brand", "note"},
}

var baseFields = []string{"fill", "ink", "pad", "gap", "typeRole", "weight", "align"}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func asString(field string, raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", errInvalid(fmt.Sprintf("%s must be a string", field), "send a string value")
	}
	return s, nil
}

func asInt(field string, raw json.RawMessage) (int, error) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil || f != math.Trunc(f) {
		return 0, errInvalid(fmt.Sprintf("%s must be an integer", field), "send an integer value")
	}
	return int(f), nil
}

// applyBaseField applies one design field shared by blocks and screens.
func applyBaseField(k string, raw json.RawMessage, fill, ink *string, pad, gap *int, role *string, weight *int, align *string) (bool, error) {
	switch k {
	case "fill", "ink":
		s, err := asString(k, raw)
		if err != nil {
			return true, err
		}
		if err := checkEnum(k, s, ColorKeys); err != nil {
			return true, err
		}
		if k == "fill" {
			*fill = s
		} else {
			*ink = s
		}
	case "pad", "gap":
		v, err := asInt(k, raw)
		if err != nil {
			return true, err
		}
		if err := checkSpace(k, v); err != nil {
			return true, err
		}
		if k == "pad" {
			*pad = v
		} else {
			*gap = v
		}
	case "typeRole":
		s, err := asString(k, raw)
		if err != nil {
			return true, err
		}
		if err := checkEnum(k, s, TypeKeys); err != nil {
			return true, err
		}
		*role = s
	case "weight":
		v, err := asInt(k, raw)
		if err != nil {
			return true, err
		}
		if err := checkWeight(k, v); err != nil {
			return true, err
		}
		*weight = v
	case "align":
		s, err := asString(k, raw)
		if err != nil {
			return true, err
		}
		if err := checkEnum(k, s, []string{"left", "center"}); err != nil {
			return true, err
		}
		*align = s
	default:
		return false, nil
	}
	return true, nil
}

func errToID() *ActionError {
	return errInv("link-target", "toId fields are set only through set_link_target so the matching edge stays in sync", "call set_link_target with the owner and toId")
}

func describe(k string, raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return k + " to " + quote(s)
	}
	return k + " to " + string(raw)
}

func (r *reducer) updateBlock() error {
	var pl struct {
		NodeID  string                     `json:"nodeId"`
		BlockID string                     `json:"blockId"`
		ChildID string                     `json:"childId,omitempty"`
		Patch   map[string]json.RawMessage `json:"patch"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	b, err := r.block(n, pl.BlockID)
	if err != nil {
		return err
	}
	if len(pl.Patch) == 0 {
		return errInvalid("patch is empty", "send the fields to change, e.g. {\"headline\":\"…\"}")
	}
	path := n.Name + " › " + BlockLabel(b.Type)
	var parts []string
	r.out.History = "web"
	if pl.ChildID != "" {
		return r.updateChild(n, b, pl.ChildID, pl.Patch, path)
	}
	for _, k := range sortedKeys(pl.Patch) {
		raw := pl.Patch[k]
		if ok, err := applyBaseField(k, raw, &b.Fill, &b.Ink, &b.Pad, &b.Gap, &b.TypeRole, &b.Weight, &b.Align); ok {
			if err != nil {
				return err
			}
			parts = append(parts, describe(k, raw))
			continue
		}
		if k == "id" || k == "type" {
			return errInvalid(k+" cannot be changed", "insert a new block of the type you need")
		}
		if k == "links" || k == "items" {
			return errInvalid(k+" are edited one child at a time", "use upsert_child / delete_child")
		}
		if k == "toId" {
			return errToID()
		}
		if !contains(blockFields[b.Type], k) {
			return errInvalid(fmt.Sprintf("%s blocks have no field %q (fields: %s)", b.Type, k, strings.Join(append(append([]string{}, baseFields...), blockFields[b.Type]...), ", ")), "call get_node to see the block")
		}
		switch k {
		case "brand", "eyebrow", "headline", "body", "heading", "text", "cite", "note":
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			switch k {
			case "brand":
				b.Brand = s
			case "eyebrow":
				b.Eyebrow = s
			case "headline":
				b.Headline = s
			case "body":
				b.Body = s
			case "heading":
				b.Heading = s
			case "text":
				b.Text = s
			case "cite":
				b.Cite = s
			case "note":
				b.Note = s
			}
			parts = append(parts, describe(k, raw))
		case "bodyRole":
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			if err := checkEnum(k, s, TypeKeys); err != nil {
				return err
			}
			b.BodyRole = s
			parts = append(parts, describe(k, raw))
		case "buttonFill", "buttonInk":
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			if err := checkEnum(k, s, ColorKeys); err != nil {
				return err
			}
			if k == "buttonFill" {
				b.ButtonFill = s
			} else {
				b.ButtonInk = s
			}
			parts = append(parts, describe(k, raw))
		case "cta", "secondary", "button":
			desc, err := r.patchBlockLink(n, b, k, raw)
			if err != nil {
				return err
			}
			parts = append(parts, desc)
		}
	}
	r.out.Summary = "Set " + path + " " + strings.Join(parts, ", ")
	return nil
}

// patchBlockLink patches hero cta/secondary or the CTA-band button: {label} or null (secondary only).
func (r *reducer) patchBlockLink(n *Node, b *Block, field string, raw json.RawMessage) (string, error) {
	if string(raw) == "null" {
		if field != "secondary" {
			return "", errInvalid(field+" cannot be removed", "change its label instead")
		}
		if b.Secondary != nil {
			for _, l := range blockLinks(n, b) {
				if l.key == b.ID+"/secondary" {
					r.clearLinks(n, []linkRef{l})
				}
			}
			b.Secondary = nil
		}
		return "removed the secondary link", nil
	}
	var lp map[string]json.RawMessage
	if err := json.Unmarshal(raw, &lp); err != nil {
		return "", errInvalid(field+" must be an object like {\"label\":\"…\"}", "send {\"label\":\"…\"}")
	}
	for k := range lp {
		if k == "toId" {
			return "", errToID()
		}
		if k != "label" {
			return "", errInvalid(fmt.Sprintf("%s accepts only label (got %q)", field, k), "send {\"label\":\"…\"}")
		}
	}
	created := false
	if field == "secondary" && b.Secondary == nil {
		b.Secondary = &Link{ID: r.newID(), Label: "Secondary"}
		created = true
	}
	if lr, ok := lp["label"]; ok {
		s, err := asString(field+".label", lr)
		if err != nil {
			return "", err
		}
		for _, l := range blockLinks(n, b) {
			if l.key == b.ID+"/"+field {
				r.relabelLink(n, l, s)
			}
		}
		if created {
			return "added secondary link " + quote(s), nil
		}
		return field + " label to " + quote(s), nil
	}
	return "added secondary link", nil
}

func (r *reducer) updateChild(n *Node, b *Block, childID string, patch map[string]json.RawMessage, path string) error {
	var parts []string
	for _, l := range nodeLinks(n) {
		if l.block == b && l.id == childID {
			for _, k := range sortedKeys(patch) {
				switch k {
				case "label":
					s, err := asString(k, patch[k])
					if err != nil {
						return err
					}
					old := *l.label
					r.relabelLink(n, l, s)
					parts = append(parts, fmt.Sprintf("%s label to %s", quote(old), quote(s)))
				case "toId":
					return errToID()
				default:
					return errInvalid(fmt.Sprintf("links accept only label (got %q)", k), "send {\"label\":\"…\"}")
				}
			}
			r.out.Summary = "Set " + path + " link " + strings.Join(parts, ", ")
			return nil
		}
	}
	for _, it := range b.Items {
		if it.ID != childID {
			continue
		}
		for _, k := range sortedKeys(patch) {
			s, err := asString(k, patch[k])
			if err != nil {
				return err
			}
			switch k {
			case "title":
				it.Title = s
			case "body":
				it.Body = s
			default:
				return errInvalid(fmt.Sprintf("feature items accept title and body (got %q)", k), "send {\"title\":\"…\"}")
			}
			parts = append(parts, describe(k, patch[k]))
		}
		r.out.Summary = "Set " + path + " › " + it.Title + " " + strings.Join(parts, ", ")
		return nil
	}
	return errUnknown("child", childID)
}

func (r *reducer) upsertChild() error {
	var pl struct {
		NodeID  string                     `json:"nodeId"`
		BlockID string                     `json:"blockId"`
		ChildID string                     `json:"childId,omitempty"`
		Fields  map[string]json.RawMessage `json:"fields,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	b, err := r.block(n, pl.BlockID)
	if err != nil {
		return err
	}
	if _, ok := pl.Fields["toId"]; ok {
		return errToID()
	}
	path := n.Name + " › " + BlockLabel(b.Type)
	r.out.History = "web"
	if pl.ChildID != "" {
		if len(pl.Fields) == 0 {
			return errInvalid("fields is empty", "send {\"label\":\"…\"} or {\"title\":\"…\"}")
		}
		return r.updateChild(n, b, pl.ChildID, pl.Fields, path)
	}
	switch b.Type {
	case "nav", "footer":
		l := &Link{ID: r.newID(), Label: "Link"}
		for k, raw := range pl.Fields {
			if k != "label" {
				return errInvalid(fmt.Sprintf("links accept only label (got %q)", k), "send {\"label\":\"…\"}")
			}
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			l.Label = s
		}
		b.Links = append(b.Links, l)
		r.out.CreatedID = l.ID
		r.out.Summary = fmt.Sprintf("Added link %s to %s", quote(l.Label), path)
	case "features":
		it := &Item{ID: r.newID(), Title: "New feature", Body: "One sentence about it."}
		for k, raw := range pl.Fields {
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			switch k {
			case "title":
				it.Title = s
			case "body":
				it.Body = s
			default:
				return errInvalid(fmt.Sprintf("feature items accept title and body (got %q)", k), "send {\"title\":\"…\"}")
			}
		}
		b.Items = append(b.Items, it)
		r.out.CreatedID = it.ID
		r.out.Summary = fmt.Sprintf("Added feature %s to %s", quote(it.Title), path)
	default:
		return errInv("child-kind", fmt.Sprintf("%s blocks have no child list (children exist on nav, footer, and features)", b.Type), "use update_block for this block's fields")
	}
	r.makeCurrent(n)
	r.selectItem(&Selection{Kind: "block", NodeID: n.ID, BlockID: b.ID, ChildID: r.out.CreatedID})
	return nil
}

func (r *reducer) deleteChild() error {
	var pl struct {
		NodeID  string `json:"nodeId"`
		BlockID string `json:"blockId"`
		ChildID string `json:"childId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	b, err := r.block(n, pl.BlockID)
	if err != nil {
		return err
	}
	path := n.Name + " › " + BlockLabel(b.Type)
	r.out.History = "web"
	for i, l := range b.Links {
		if l.ID == pl.ChildID {
			for _, ref := range blockLinks(n, b) {
				if ref.id == l.ID {
					r.clearLinks(n, []linkRef{ref})
				}
			}
			b.Links = append(b.Links[:i], b.Links[i+1:]...)
			r.out.Summary = fmt.Sprintf("Deleted link %s from %s", quote(l.Label), path)
			return nil
		}
	}
	for i, it := range b.Items {
		if it.ID == pl.ChildID {
			b.Items = append(b.Items[:i], b.Items[i+1:]...)
			r.out.Summary = fmt.Sprintf("Deleted feature %s from %s", quote(it.Title), path)
			return nil
		}
	}
	if (b.CTA != nil && b.CTA.ID == pl.ChildID) || (b.Button != nil && b.Button.ID == pl.ChildID) {
		return errInvalid("a block's main button cannot be deleted", "delete the block, or clear its target with set_link_target")
	}
	if b.Secondary != nil && b.Secondary.ID == pl.ChildID {
		return errInvalid("remove the secondary link with update_block {secondary: null}", "call update_block")
	}
	return errUnknown("child", pl.ChildID)
}

func (r *reducer) reorderBlocks() error {
	var pl struct {
		NodeID   string   `json:"nodeId"`
		BlockIDs []string `json:"blockIds"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	var have []string
	for _, b := range n.Blocks {
		have = append(have, b.ID)
	}
	if !samePermutation(have, pl.BlockIDs) {
		return errInvalid(fmt.Sprintf("blockIds must list exactly the blocks of %s: %s", n.ID, strings.Join(have, ", ")), "send every block id once")
	}
	nb := make([]*Block, 0, len(n.Blocks))
	for _, id := range pl.BlockIDs {
		nb = append(nb, n.Block(id))
	}
	n.Blocks = nb
	r.out.History = "web"
	r.out.Summary = "Reordered blocks on " + n.Name
	return nil
}

func (r *reducer) duplicateBlock() error {
	var pl struct {
		NodeID  string `json:"nodeId"`
		BlockID string `json:"blockId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	b, err := r.block(n, pl.BlockID)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(b)
	var c Block
	_ = json.Unmarshal(raw, &c)
	c.ID = r.newID()
	for _, l := range c.Links {
		l.ID = r.newID()
	}
	for _, it := range c.Items {
		it.ID = r.newID()
	}
	for _, l := range []*Link{c.CTA, c.Secondary, c.Button} {
		if l != nil {
			l.ID = r.newID()
		}
	}
	idx := blockIndex(n, b.ID) + 1
	n.Blocks = append(n.Blocks[:idx], append([]*Block{&c}, n.Blocks[idx:]...)...)
	for _, l := range blockLinks(n, &c) {
		if to := string(*l.toID); to != "" && to != n.ID {
			if tn := r.p.Node(to); tn != nil {
				r.ensureEdge(n, tn, *l.label)
			}
		}
	}
	r.selectItem(&Selection{Kind: "block", NodeID: n.ID, BlockID: c.ID})
	r.out.CreatedID = c.ID
	r.out.History = "web"
	r.out.Summary = fmt.Sprintf("Duplicated %s › %s", n.Name, BlockLabel(b.Type))
	return nil
}

func (r *reducer) deleteBlock() error {
	var pl struct {
		NodeID  string `json:"nodeId"`
		BlockID string `json:"blockId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.page(pl.NodeID)
	if err != nil {
		return err
	}
	b, err := r.block(n, pl.BlockID)
	if err != nil {
		return err
	}
	refs := blockLinks(n, b)
	idx := blockIndex(n, b.ID)
	n.Blocks = append(n.Blocks[:idx], n.Blocks[idx+1:]...)
	// links of a removed block no longer own edges
	for _, l := range refs {
		if to := string(*l.toID); to != "" && to != n.ID {
			r.dropEdgeIfUnowned(n.ID, to, *l.label)
		}
	}
	pins := 0
	kept := r.p.Pins[:0]
	for _, x := range r.p.Pins {
		if x.Target.Type == "node" && x.Target.BlockID == b.ID {
			pins++
			continue
		}
		kept = append(kept, x)
	}
	r.p.Pins = kept
	r.out.History = "web"
	r.out.Summary = fmt.Sprintf("Deleted %s › %s (%d pin(s) removed)", n.Name, BlockLabel(b.Type), pins)
	return nil
}

// ---- screens ----

func (r *reducer) updateScreen() error {
	var pl struct {
		NodeID string                     `json:"nodeId"`
		Patch  map[string]json.RawMessage `json:"patch"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.screen(pl.NodeID)
	if err != nil {
		return err
	}
	if len(pl.Patch) == 0 {
		return errInvalid("patch is empty", "send the fields to change, e.g. {\"title\":\"…\"}")
	}
	var parts []string
	for _, k := range sortedKeys(pl.Patch) {
		raw := pl.Patch[k]
		if ok, err := applyBaseField(k, raw, &n.Fill, &n.Ink, &n.Pad, &n.Gap, &n.TypeRole, &n.Weight, &n.Align); ok {
			if err != nil {
				return err
			}
			parts = append(parts, describe(k, raw))
			continue
		}
		switch k {
		case "name", "title", "body":
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			switch k {
			case "name":
				if strings.TrimSpace(s) == "" {
					return errInvalid("name cannot be empty", "send a non-empty name")
				}
				n.Name = s
			case "title":
				n.Title = s
			case "body":
				n.Body = s
			}
			parts = append(parts, describe(k, raw))
		case "archetype":
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			if err := checkEnum(k, s, archetypes); err != nil {
				return err
			}
			n.Archetype = s
			parts = append(parts, describe(k, raw))
		case "layout":
			s, err := asString(k, raw)
			if err != nil {
				return err
			}
			if err := checkEnum(k, s, []string{"center", "bottom", "split"}); err != nil {
				return err
			}
			n.Layout = s
			parts = append(parts, describe(k, raw))
		case "cta":
			d, err := r.patchScreenCTA(n, raw)
			if err != nil {
				return err
			}
			parts = append(parts, d)
		case "hotspots":
			return errInvalid("hotspots are edited one at a time", "use upsert_hotspot / delete_hotspot")
		case "toId":
			return errToID()
		case "id", "kind", "flowId":
			return errInvalid(k+" cannot be changed with update_screen", "use upsert_node to move a screen to another flow")
		default:
			return errInvalid(fmt.Sprintf("screens have no field %q", k), "call get_node to see the screen")
		}
	}
	r.out.History = "story"
	r.out.Summary = "Set " + n.Name + " " + strings.Join(parts, ", ")
	return nil
}

func (r *reducer) patchScreenCTA(n *Node, raw json.RawMessage) (string, error) {
	if string(raw) == "null" {
		if n.CTA != nil {
			for _, l := range nodeLinks(n) {
				if l.key == "cta" {
					r.clearLinks(n, []linkRef{l})
				}
			}
			n.CTA = nil
		}
		return "removed the CTA", nil
	}
	var lp map[string]json.RawMessage
	if err := json.Unmarshal(raw, &lp); err != nil {
		return "", errInvalid("cta must be null or an object {label?, size?, fill?, ink?}", "send {\"size\":\"md\"}")
	}
	if n.CTA == nil {
		n.CTA = &ScreenCTA{Label: "Continue", Size: "md", Fill: "accent", Ink: "paper"}
	}
	var parts []string
	for _, k := range sortedKeys(lp) {
		v := lp[k]
		switch k {
		case "toId":
			return "", errToID()
		case "label":
			s, err := asString("cta.label", v)
			if err != nil {
				return "", err
			}
			for _, l := range nodeLinks(n) {
				if l.key == "cta" {
					r.relabelLink(n, l, s)
				}
			}
		case "size":
			s, err := asString("cta.size", v)
			if err != nil {
				return "", err
			}
			if err := checkEnum("cta.size", s, []string{"sm", "md", "lg"}); err != nil {
				return "", err
			}
			n.CTA.Size = s
		case "fill", "ink":
			s, err := asString("cta."+k, v)
			if err != nil {
				return "", err
			}
			if err := checkEnum("cta."+k, s, ColorKeys); err != nil {
				return "", err
			}
			if k == "fill" {
				n.CTA.Fill = s
			} else {
				n.CTA.Ink = s
			}
		default:
			return "", errInvalid(fmt.Sprintf("cta has no field %q (fields: label, size, fill, ink)", k), "send a valid cta field")
		}
		parts = append(parts, describe("cta "+k, v))
	}
	return strings.Join(parts, ", "), nil
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func (r *reducer) upsertHotspot() error {
	var pl struct {
		NodeID    string   `json:"nodeId"`
		HotspotID string   `json:"hotspotId,omitempty"`
		Label     *string  `json:"label,omitempty"`
		X         *float64 `json:"x,omitempty"`
		Y         *float64 `json:"y,omitempty"`
		W         *float64 `json:"w,omitempty"`
		H         *float64 `json:"h,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.screen(pl.NodeID)
	if err != nil {
		return err
	}
	var h *Hotspot
	created := false
	if pl.HotspotID == "" {
		h = &Hotspot{ID: r.newID(), Label: "Hotspot", X: 145, Y: 400, W: 100, H: 44}
		n.Hotspots = append(n.Hotspots, h)
		created = true
	} else if h = n.Hotspot(pl.HotspotID); h == nil {
		return errUnknown("hotspot", pl.HotspotID)
	}
	if pl.X != nil {
		h.X = *pl.X
	}
	if pl.Y != nil {
		h.Y = *pl.Y
	}
	if pl.W != nil {
		h.W = *pl.W
	}
	if pl.H != nil {
		h.H = *pl.H
	}
	h.W = clamp(math.Round(h.W), 8, 390)
	h.H = clamp(math.Round(h.H), 8, 844)
	h.X = clamp(math.Round(h.X), 0, 390-h.W)
	h.Y = clamp(math.Round(h.Y), 0, 844-h.H)
	if pl.Label != nil {
		if created {
			h.Label = *pl.Label
		} else {
			for _, l := range nodeLinks(n) {
				if l.key == h.ID {
					r.relabelLink(n, l, *pl.Label)
				}
			}
		}
	}
	r.out.History = "story"
	if created {
		r.out.CreatedID = h.ID
		r.makeCurrent(n)
		r.selectItem(&Selection{Kind: "hotspot", NodeID: n.ID, HotspotID: h.ID})
		r.out.Summary = fmt.Sprintf("Added hotspot %s to %s", quote(h.Label), n.Name)
	} else {
		r.out.Summary = fmt.Sprintf("Updated hotspot %s on %s (%g,%g %g×%g)", quote(h.Label), n.Name, h.X, h.Y, h.W, h.H)
	}
	return nil
}

func (r *reducer) deleteHotspot() error {
	var pl struct {
		NodeID    string `json:"nodeId"`
		HotspotID string `json:"hotspotId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.screen(pl.NodeID)
	if err != nil {
		return err
	}
	h := n.Hotspot(pl.HotspotID)
	if h == nil {
		return errUnknown("hotspot", pl.HotspotID)
	}
	for _, l := range nodeLinks(n) {
		if l.key == h.ID {
			r.clearLinks(n, []linkRef{l})
		}
	}
	kept := n.Hotspots[:0]
	for _, x := range n.Hotspots {
		if x.ID != h.ID {
			kept = append(kept, x)
		}
	}
	n.Hotspots = kept
	r.out.History = "story"
	r.out.Summary = fmt.Sprintf("Deleted hotspot %s from %s", quote(h.Label), n.Name)
	return nil
}

// ---- link targets and edges ----

type LinkOwner struct {
	BlockID   string `json:"blockId,omitempty"`
	ChildID   string `json:"childId,omitempty"`
	Field     string `json:"field,omitempty"`
	CTA       bool   `json:"cta,omitempty"`
	HotspotID string `json:"hotspotId,omitempty"`
}

func (r *reducer) setLinkTargetA() error {
	var pl struct {
		NodeID string          `json:"nodeId"`
		Owner  LinkOwner       `json:"owner"`
		ToID   json.RawMessage `json:"toId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	n, err := r.node(pl.NodeID)
	if err != nil {
		return err
	}
	var to Ref
	if pl.ToID == nil {
		return errInvalid("toId is required (a node id, or null to clear)", "send toId")
	}
	if err := json.Unmarshal(pl.ToID, &to); err != nil {
		return errInvalid("toId must be a node id string or null", "send toId")
	}
	key := ""
	o := pl.Owner
	switch {
	case o.HotspotID != "":
		key = o.HotspotID
		if n.Kind != "screen" || n.Hotspot(o.HotspotID) == nil {
			return errUnknown("hotspot", o.HotspotID)
		}
	case o.CTA:
		if n.Kind != "screen" {
			return errInvalid("owner {cta:true} applies to mobile screens", "for pages use {blockId, field:'cta'|'secondary'|'button'}")
		}
		if n.CTA == nil {
			return errInv("no-cta", n.Name+" has no CTA", "add one with update_screen {cta:{label:\"…\"}}")
		}
		key = "cta"
	case o.BlockID != "" && o.ChildID != "":
		if _, err := r.block(n, o.BlockID); err != nil {
			return err
		}
		key = o.BlockID + "/" + o.ChildID
	case o.BlockID != "" && o.Field != "":
		b, err := r.block(n, o.BlockID)
		if err != nil {
			return err
		}
		if err := checkEnum("owner.field", o.Field, []string{"cta", "secondary", "button"}); err != nil {
			return err
		}
		if o.Field == "secondary" && b.Secondary == nil {
			return errInv("no-secondary", n.Name+" › "+BlockLabel(b.Type)+" has no secondary link", "add one with update_block {secondary:{label:\"…\"}}")
		}
		key = o.BlockID + "/" + o.Field
	default:
		return errInvalid("owner must be {blockId, childId}, {blockId, field}, {cta: true}, or {hotspotId}", "call get_node to find the owner")
	}
	var ref *linkRef
	for _, l := range nodeLinks(n) {
		if l.key == key {
			l := l
			ref = &l
		}
	}
	if ref == nil {
		return errUnknown("link owner", key)
	}
	if err := r.setLinkTarget(n, *ref, string(to)); err != nil {
		return err
	}
	r.out.History = histFor(n.Channel())
	if to == "" {
		r.out.Summary = fmt.Sprintf("Cleared the target of %s › %s", n.Name, ref.name)
	} else if string(to) == n.ID {
		r.out.Summary = fmt.Sprintf("Linked %s › %s to its own page (no edge)", n.Name, ref.name)
	} else {
		r.out.Summary = fmt.Sprintf("Linked %s › %s to %s", n.Name, ref.name, r.nodeName(string(to)))
	}
	return nil
}

func (r *reducer) upsertEdge() error {
	var pl struct {
		ID     string  `json:"id,omitempty"`
		FromID string  `json:"fromId,omitempty"`
		ToID   string  `json:"toId,omitempty"`
		Label  *string `json:"label,omitempty"`
		Kind   string  `json:"kind,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if pl.Kind != "" {
		if err := checkEnum("kind", pl.Kind, []string{"primary", "alt", "back", "cross"}); err != nil {
			return err
		}
	}
	if pl.ID == "" {
		from, err := r.node(pl.FromID)
		if err != nil {
			return err
		}
		to, err := r.node(pl.ToID)
		if err != nil {
			return err
		}
		label := "Link"
		if pl.Label != nil {
			label = *pl.Label
		}
		kind, err := r.validateEdge(from, to, label, pl.Kind, "")
		if err != nil {
			return err
		}
		e := &Edge{ID: r.newID(), FromID: from.ID, ToID: to.ID, Label: label, Kind: kind}
		r.p.Edges = append(r.p.Edges, e)
		r.selectItem(&Selection{Kind: "edge", EdgeID: e.ID})
		r.out.CreatedID = e.ID
		r.out.History = histFor(from.Channel())
		r.out.Summary = fmt.Sprintf("Connected %s → %s %s (%s)", from.Name, to.Name, quote(label), kind)
		return nil
	}
	e := r.p.Edge(pl.ID)
	if e == nil {
		return errUnknown("edge", pl.ID)
	}
	owners := r.edgeOwners(e)
	if pl.FromID != "" && pl.FromID != e.FromID && len(owners) > 0 {
		return errInv("link-edge", fmt.Sprintf("edge %s is owned by link %s on %s, so its source cannot move", e.ID, *owners[0].label, r.nodeName(e.FromID)), "clear that link with set_link_target and create a new edge")
	}
	fromID, toID, label := e.FromID, e.ToID, e.Label
	if pl.FromID != "" {
		fromID = pl.FromID
	}
	if pl.ToID != "" {
		toID = pl.ToID
	}
	if pl.Label != nil {
		label = *pl.Label
	}
	from, err := r.node(fromID)
	if err != nil {
		return err
	}
	to, err := r.node(toID)
	if err != nil {
		return err
	}
	kind := pl.Kind
	if kind == "" {
		kind = e.Kind
		if from.FlowID != to.FlowID {
			kind = "cross"
		} else if kind == "cross" {
			kind = "primary"
		}
	}
	kind, err = r.validateEdge(from, to, label, kind, e.ID)
	if err != nil {
		return err
	}
	for _, o := range owners {
		*o.toID = Ref(toID)
		*o.label = label
	}
	e.FromID, e.ToID, e.Label, e.Kind = fromID, toID, label, kind
	r.out.History = histFor(from.Channel())
	r.out.Summary = fmt.Sprintf("Edge %s → %s %s (%s)", from.Name, to.Name, quote(label), kind)
	return nil
}

// validateEdge checks §6.1 for a prospective edge and returns its kind.
func (r *reducer) validateEdge(from, to *Node, label, kind, selfID string) (string, error) {
	if from.Channel() != to.Channel() {
		return "", errSameChannel(from, to)
	}
	if from.ID == to.ID {
		return "", errInv("no-self-edge", fmt.Sprintf("an edge cannot connect %s to itself", from.ID), "pick a different toId")
	}
	if ex := r.findEdge(from.ID, to.ID, label); ex != nil && ex.ID != selfID {
		return "", errInv("unique-edge", fmt.Sprintf("edge %s already connects %s → %s with label %s", ex.ID, from.Name, to.Name, quote(label)), "use a different label for a branch, or update "+ex.ID)
	}
	cross := from.FlowID != to.FlowID
	if cross {
		if kind != "" && kind != "cross" {
			return "", errInv("cross-kind", fmt.Sprintf("%s and %s are in different flows, so the edge kind is cross (read-only)", from.Name, to.Name), "omit kind")
		}
		return "cross", nil
	}
	if kind == "cross" {
		return "", errInv("cross-kind", fmt.Sprintf("%s and %s are in the same flow, so the kind must be primary, alt, or back", from.Name, to.Name), "send kind primary, alt, or back")
	}
	if kind == "" {
		kind = "primary"
	}
	return kind, nil
}

func (r *reducer) deleteEdgeA() error {
	var pl struct {
		EdgeID string `json:"edgeId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	e := r.p.Edge(pl.EdgeID)
	if e == nil {
		return errUnknown("edge", pl.EdgeID)
	}
	owners := r.edgeOwners(e)
	for _, o := range owners {
		*o.toID = ""
	}
	r.removeEdge(e.ID)
	ch := ChanWeb
	if from := r.p.Node(e.FromID); from != nil {
		ch = from.Channel()
	}
	r.out.History = histFor(ch)
	r.out.Summary = fmt.Sprintf("Deleted edge %s → %s %s", r.nodeName(e.FromID), r.nodeName(e.ToID), quote(e.Label))
	if len(owners) > 0 {
		r.out.Summary += fmt.Sprintf(" and cleared %d link target(s)", len(owners))
	}
	return nil
}

// ---- draw ----

var strokeWidths = []float64{1, 1.5, 2, 3}

func validateShape(s *Shape) error {
	if err := checkEnum("shape.type", s.Type, ShapeTypes); err != nil {
		return err
	}
	switch s.Type {
	case "rect", "ellipse":
		if s.W < 0 {
			s.X += s.W
			s.W = -s.W
		}
		if s.H < 0 {
			s.Y += s.H
			s.H = -s.H
		}
		if s.W == 0 || s.H == 0 {
			return errInvalid("rect and ellipse need w and h greater than 0", "send a size")
		}
		if s.Fill == "" {
			s.Fill = "none"
		}
		if s.Fill != "none" {
			if err := checkEnum("fill", s.Fill, ColorKeys); err != nil {
				return err
			}
		}
	case "text":
		if s.Role == "" {
			s.Role = "md"
		}
		if s.Ink == "" {
			s.Ink = "ink"
		}
		if err := checkEnum("role", s.Role, TypeKeys); err != nil {
			return err
		}
		if err := checkEnum("ink", s.Ink, ColorKeys); err != nil {
			return err
		}
		return nil
	case "pen":
		if len(s.Points) < 2 {
			return errInvalid("pen needs at least 2 points", "send points [[x,y],…]")
		}
	}
	if s.Stroke == "" {
		s.Stroke = "ink"
	}
	if err := checkEnum("stroke", s.Stroke, ColorKeys); err != nil {
		return err
	}
	if s.SW == 0 {
		s.SW = 1.5
	}
	ok := false
	for _, w := range strokeWidths {
		if s.SW == w {
			ok = true
		}
	}
	if !ok {
		return errInvalid(fmt.Sprintf("sw must be 1, 1.5, 2, or 3 (got %g)", s.SW), "send a valid stroke width")
	}
	return nil
}

func (r *reducer) addShape() error {
	var pl struct {
		Shape json.RawMessage `json:"shape"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	var s Shape
	dec := json.NewDecoder(bytes.NewReader(pl.Shape))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return errInvalid("bad shape: "+err.Error(), "send {type, …fields of that type}")
	}
	if s.ID != "" {
		return errInvalid("shape ids are generated by the Store", "omit id")
	}
	if err := validateShape(&s); err != nil {
		return err
	}
	s.ID = r.newID()
	r.p.Draw.Shapes = append(r.p.Draw.Shapes, &s)
	r.selectItem(&Selection{Kind: "shape", ShapeID: s.ID})
	r.out.CreatedID = s.ID
	r.out.History = "draw"
	r.out.Summary = "Added a " + s.Type
	if s.Type == "text" {
		r.out.Summary += " " + quote(s.Text)
	}
	return nil
}

var shapeFields = map[string][]string{
	"rect":    {"x", "y", "w", "h", "stroke", "fill", "sw"},
	"ellipse": {"x", "y", "w", "h", "stroke", "fill", "sw"},
	"line":    {"x1", "y1", "x2", "y2", "stroke", "sw"},
	"arrow":   {"x1", "y1", "x2", "y2", "stroke", "sw"},
	"text":    {"x", "y", "text", "role", "ink"},
	"pen":     {"points", "stroke", "sw"},
}

func (r *reducer) updateShape() error {
	var pl struct {
		ShapeID string                     `json:"shapeId"`
		Patch   map[string]json.RawMessage `json:"patch"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	s := r.p.Shape(pl.ShapeID)
	if s == nil {
		return errUnknown("shape", pl.ShapeID)
	}
	if len(pl.Patch) == 0 {
		return errInvalid("patch is empty", "send the fields to change")
	}
	for k := range pl.Patch {
		if !contains(shapeFields[s.Type], k) {
			return errInvalid(fmt.Sprintf("%s shapes have no editable field %q (fields: %s)", s.Type, k, strings.Join(shapeFields[s.Type], ", ")), "send a valid field")
		}
	}
	raw, _ := json.Marshal(s)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	for k, v := range pl.Patch {
		m[k] = v
	}
	merged, _ := json.Marshal(m)
	var ns Shape
	if err := json.Unmarshal(merged, &ns); err != nil {
		return errInvalid("bad shape patch: "+err.Error(), "send numbers for coordinates and token keys for colors")
	}
	if err := validateShape(&ns); err != nil {
		return err
	}
	*s = ns
	r.out.History = "draw"
	r.out.Summary = "Updated " + s.Type + " " + strings.Join(sortedKeys(pl.Patch), ", ")
	return nil
}

func (r *reducer) duplicateShape() error {
	var pl struct {
		ShapeID string `json:"shapeId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	s := r.p.Shape(pl.ShapeID)
	if s == nil {
		return errUnknown("shape", pl.ShapeID)
	}
	c := *s
	c.ID = r.newID()
	c.X += 16
	c.Y += 16
	c.X1 += 16
	c.Y1 += 16
	c.X2 += 16
	c.Y2 += 16
	c.Points = nil
	for _, pt := range s.Points {
		c.Points = append(c.Points, [2]float64{pt[0] + 16, pt[1] + 16})
	}
	r.p.Draw.Shapes = append(r.p.Draw.Shapes, &c)
	r.selectItem(&Selection{Kind: "shape", ShapeID: c.ID})
	r.out.CreatedID = c.ID
	r.out.History = "draw"
	r.out.Summary = "Duplicated a " + s.Type
	return nil
}

func (r *reducer) deleteShape() error {
	var pl struct {
		ShapeID string `json:"shapeId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	s := r.p.Shape(pl.ShapeID)
	if s == nil {
		return errUnknown("shape", pl.ShapeID)
	}
	kept := r.p.Draw.Shapes[:0]
	for _, x := range r.p.Draw.Shapes {
		if x.ID != s.ID {
			kept = append(kept, x)
		}
	}
	r.p.Draw.Shapes = kept
	r.out.History = "draw"
	r.out.Summary = "Deleted a " + s.Type
	return nil
}

// ---- doc ----

func (r *reducer) setDoc() error {
	var pl struct {
		Markdown *string `json:"markdown"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if pl.Markdown == nil {
		return errInvalid("markdown is required", "send the full markdown text")
	}
	r.p.Doc.Markdown = *pl.Markdown
	r.out.History = "doc"
	r.out.Summary = fmt.Sprintf("Updated DESIGN.md (%d words)", len(strings.Fields(*pl.Markdown)))
	return nil
}

// ---- pins ----

var pinKinds = []string{"issue", "suggestion", "question"}

func (r *reducer) pinHistory(t PinTarget) string {
	if t.Type == "draw" {
		return "draw"
	}
	if n := r.p.Node(t.NodeID); n != nil {
		return histFor(n.Channel())
	}
	return "web"
}

// PinWhere is the human target label of a pin ("Home › Hero", "Storyboard › Cart", "Draw").
func PinWhere(p *Project, x *Pin) string {
	switch x.Target.Type {
	case "draw":
		return "Draw"
	case "board":
		if n := p.Node(x.Target.NodeID); n != nil {
			return "Storyboard › " + n.Name
		}
	default:
		if n := p.Node(x.Target.NodeID); n != nil {
			if b := n.Block(x.Target.BlockID); b != nil {
				return n.Name + " › " + BlockLabel(b.Type)
			}
			return n.Name
		}
	}
	return "?"
}

func (r *reducer) addPin() error {
	var pl struct {
		Kind   string    `json:"kind"`
		Body   string    `json:"body"`
		Target PinTarget `json:"target"`
		X      float64   `json:"x"`
		Y      float64   `json:"y"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	if pl.Kind == "" {
		pl.Kind = "issue"
	}
	if err := checkEnum("kind", pl.Kind, pinKinds); err != nil {
		return err
	}
	body := strings.TrimSpace(pl.Body)
	if body == "" {
		return errInv("empty-pin", "a pin needs a non-empty body", "send the note text in body")
	}
	t := pl.Target
	switch t.Type {
	case "node":
		n, err := r.node(t.NodeID)
		if err != nil {
			return err
		}
		if t.BlockID != "" {
			if n.Kind != "page" || n.Block(t.BlockID) == nil {
				return errUnknown("block", t.BlockID)
			}
		}
		pl.X, pl.Y = clamp(pl.X, 0, 1), clamp(pl.Y, 0, 1)
	case "board":
		if _, err := r.node(t.NodeID); err != nil {
			return err
		}
		t.BlockID = ""
		pl.X, pl.Y = clamp(pl.X, 0, 1), clamp(pl.Y, 0, 1)
	case "draw":
		t.NodeID, t.BlockID = "", ""
	default:
		return errInvalid("target.type must be node, board, or draw", "send {type:'node', nodeId, blockId?}, {type:'board', nodeId}, or {type:'draw'}")
	}
	r.p.PinCounter++
	x := &Pin{ID: r.newID(), N: r.p.PinCounter, Kind: pl.Kind, Body: body, Target: t, X: pl.X, Y: pl.Y, CreatedAt: r.now.UnixMilli()}
	r.p.Pins = append(r.p.Pins, x)
	r.selectItem(&Selection{Kind: "pin", PinID: x.ID})
	r.out.CreatedID = x.ID
	r.out.History = r.pinHistory(t)
	r.out.Summary = fmt.Sprintf("Added %s pin %d on %s", x.Kind, x.N, PinWhere(r.p, x))
	return nil
}

func (r *reducer) updatePin() error {
	var pl struct {
		PinID    string   `json:"pinId"`
		Kind     *string  `json:"kind,omitempty"`
		Body     *string  `json:"body,omitempty"`
		Resolved *bool    `json:"resolved,omitempty"`
		X        *float64 `json:"x,omitempty"`
		Y        *float64 `json:"y,omitempty"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	x := r.p.Pin(pl.PinID)
	if x == nil {
		return errUnknown("pin", pl.PinID)
	}
	var parts []string
	if pl.Kind != nil {
		if err := checkEnum("kind", *pl.Kind, pinKinds); err != nil {
			return err
		}
		x.Kind = *pl.Kind
		parts = append(parts, "kind "+x.Kind)
	}
	if pl.Body != nil {
		b := strings.TrimSpace(*pl.Body)
		if b == "" {
			return errInv("empty-pin", "a pin body cannot be empty", "send text, or delete the pin with delete_pin")
		}
		x.Body = b
		parts = append(parts, "note")
	}
	if pl.Resolved != nil {
		x.Resolved = *pl.Resolved
		if x.Resolved {
			parts = append(parts, "resolved")
		} else {
			parts = append(parts, "reopened")
		}
	}
	if pl.X != nil || pl.Y != nil {
		if pl.X != nil {
			x.X = *pl.X
		}
		if pl.Y != nil {
			x.Y = *pl.Y
		}
		if x.Target.Type != "draw" {
			x.X, x.Y = clamp(x.X, 0, 1), clamp(x.Y, 0, 1)
		}
		parts = append(parts, "moved")
	}
	if len(parts) == 0 {
		return errInvalid("nothing to change", "send kind, body, resolved, x, or y")
	}
	r.out.History = r.pinHistory(x.Target)
	r.out.Summary = fmt.Sprintf("Pin %d (%s): %s", x.N, PinWhere(r.p, x), strings.Join(parts, ", "))
	return nil
}

func (r *reducer) deletePin() error {
	var pl struct {
		PinID string `json:"pinId"`
	}
	if err := r.decode(&pl); err != nil {
		return err
	}
	x := r.p.Pin(pl.PinID)
	if x == nil {
		return errUnknown("pin", pl.PinID)
	}
	r.out.History = r.pinHistory(x.Target)
	kept := r.p.Pins[:0]
	for _, y := range r.p.Pins {
		if y.ID != x.ID {
			kept = append(kept, y)
		}
	}
	r.p.Pins = kept
	r.out.Summary = fmt.Sprintf("Deleted pin %d", x.N)
	return nil
}

// ---- view repair ----

func defaultNode(p *Project, ch string) string {
	for _, f := range p.FlowsOf(ch) {
		if f.StartID != "" {
			return string(f.StartID)
		}
	}
	for _, n := range p.Nodes {
		if n.Channel() == ch {
			return n.ID
		}
	}
	return ""
}

func selectionValid(p *Project, s *Selection) bool {
	if s == nil {
		return true
	}
	switch s.Kind {
	case "node":
		return p.Node(s.NodeID) != nil
	case "block":
		n := p.Node(s.NodeID)
		if n == nil {
			return false
		}
		b := n.Block(s.BlockID)
		return b != nil && (s.ChildID == "" || blockHasChild(b, s.ChildID))
	case "hotspot":
		n := p.Node(s.NodeID)
		return n != nil && n.Hotspot(s.HotspotID) != nil
	case "edge":
		return p.Edge(s.EdgeID) != nil
	case "shape":
		return p.Shape(s.ShapeID) != nil
	case "pin":
		return p.Pin(s.PinID) != nil
	}
	return false
}

// fixView keeps the shared view pointing at things that exist.
func fixView(p *Project, v *SharedView, tabSel map[string]*Selection) {
	fix := func(cur *Ref, ch string) {
		if *cur != "" {
			if n := p.Node(string(*cur)); n == nil || n.Channel() != ch {
				*cur = ""
			}
		}
		if *cur == "" {
			*cur = Ref(defaultNode(p, ch))
		}
	}
	fix(&v.CurrentNodeID.Web, ChanWeb)
	fix(&v.CurrentNodeID.Mobile, ChanMobile)
	if !selectionValid(p, v.Selection) {
		v.Selection = nil
	}
	for k, s := range tabSel {
		if !selectionValid(p, s) {
			delete(tabSel, k)
		}
	}
	if !contains(Tabs, v.Tab) {
		v.Tab = "web"
	}
	if v.View.Web != "board" {
		v.View.Web = "page"
	}
	if v.View.Story != "board" {
		v.View.Story = "page"
	}
	if _, ok := Breakpoints[v.Breakpoint]; !ok {
		v.Breakpoint = "desktop"
	}
}
