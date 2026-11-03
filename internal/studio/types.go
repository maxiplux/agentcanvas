// Package studio owns the AgentCanvas project model (§6), the seed (§7),
// the reducer with every §6.1 invariant, histories (§11.3), checks (§9),
// lane order (§9.5) and persistence (§12.1).
package studio

import (
	"encoding/json"
)

// Ref is a nullable id. The empty string marshals as JSON null.
type Ref string

func (r Ref) MarshalJSON() ([]byte, error) {
	if r == "" {
		return []byte("null"), nil
	}
	return json.Marshal(string(r))
}

func (r *Ref) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*r = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*r = Ref(s)
	return nil
}

// Channel values.
const (
	ChanWeb    = "web"
	ChanMobile = "mobile"
)

// Tabs.
var Tabs = []string{"web", "story", "draw", "doc"}

// Token keys.
var (
	ColorKeys  = []string{"paper", "ink", "muted", "accent", "wash", "line"}
	TypeKeys   = []string{"xs", "sm", "md", "lg", "xl", "display"}
	SpaceScale = []int{0, 4, 8, 16, 24, 32, 48, 64}
)

type Project struct {
	Version    int         `json:"version"`
	Tokens     Tokens      `json:"tokens"`
	Flows      []*Flow     `json:"flows"`
	Nodes      []*Node     `json:"nodes"`
	Edges      []*Edge     `json:"edges"`
	Draw       Draw        `json:"draw"`
	Canvas     CanvasState `json:"canvas"`
	Doc        Doc         `json:"doc"`
	Pins       []*Pin      `json:"pins"`
	PinCounter int         `json:"pinCounter"`
}

type Tokens struct {
	Colors map[string]string   `json:"colors"`
	Type   map[string]TypeSpec `json:"type"`
	Space  []int               `json:"space"`
}

type TypeSpec struct {
	Size   int `json:"size"`
	LH     int `json:"lh"`
	Weight int `json:"weight"`
}

// Point is a manual artboard position on the canvas, in world units (§8.2).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Flow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Channel string `json:"channel"`
	StartID Ref    `json:"startId"`
}

// Node is a WebPage (kind "page") or a MobileScreen (kind "screen").
// MarshalJSON emits only the fields of its kind.
type Node struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	FlowID string `json:"flowId"`
	Name   string `json:"name"`
	Pos    *Point `json:"pos,omitempty"` // nil = auto-placed on the canvas
	// page
	Blocks []*Block `json:"blocks"`
	// screen
	Archetype string     `json:"archetype"`
	Layout    string     `json:"layout"`
	Fill      string     `json:"fill"`
	Ink       string     `json:"ink"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	TypeRole  string     `json:"typeRole"`
	Weight    int        `json:"weight"`
	Align     string     `json:"align"`
	Pad       int        `json:"pad"`
	Gap       int        `json:"gap"`
	CTA       *ScreenCTA `json:"cta"`
	Hotspots  []*Hotspot `json:"hotspots"`
}

// Channel returns the node's channel, derived from its kind.
func (n *Node) Channel() string {
	if n.Kind == "screen" {
		return ChanMobile
	}
	return ChanWeb
}

type pageJSON struct {
	Kind   string   `json:"kind"`
	ID     string   `json:"id"`
	FlowID string   `json:"flowId"`
	Name   string   `json:"name"`
	Pos    *Point   `json:"pos,omitempty"`
	Blocks []*Block `json:"blocks"`
}

type screenJSON struct {
	Kind      string     `json:"kind"`
	ID        string     `json:"id"`
	FlowID    string     `json:"flowId"`
	Name      string     `json:"name"`
	Pos       *Point     `json:"pos,omitempty"`
	Archetype string     `json:"archetype"`
	Layout    string     `json:"layout"`
	Fill      string     `json:"fill"`
	Ink       string     `json:"ink"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	TypeRole  string     `json:"typeRole"`
	Weight    int        `json:"weight"`
	Align     string     `json:"align"`
	Pad       int        `json:"pad"`
	Gap       int        `json:"gap"`
	CTA       *ScreenCTA `json:"cta"`
	Hotspots  []*Hotspot `json:"hotspots"`
}

func (n *Node) MarshalJSON() ([]byte, error) {
	if n.Kind == "screen" {
		hs := n.Hotspots
		if hs == nil {
			hs = []*Hotspot{}
		}
		return json.Marshal(screenJSON{n.Kind, n.ID, n.FlowID, n.Name, n.Pos, n.Archetype, n.Layout, n.Fill, n.Ink,
			n.Title, n.Body, n.TypeRole, n.Weight, n.Align, n.Pad, n.Gap, n.CTA, hs})
	}
	bl := n.Blocks
	if bl == nil {
		bl = []*Block{}
	}
	return json.Marshal(pageJSON{n.Kind, n.ID, n.FlowID, n.Name, n.Pos, bl})
}

type ScreenCTA struct {
	Label string `json:"label"`
	ToID  Ref    `json:"toId"`
	Size  string `json:"size"`
	Fill  string `json:"fill"`
	Ink   string `json:"ink"`
}

type Hotspot struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	ToID  Ref     `json:"toId"`
}

type Edge struct {
	ID     string `json:"id"`
	FromID string `json:"fromId"`
	ToID   string `json:"toId"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
}

type Link struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	ToID  Ref    `json:"toId"`
}

type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Block is any web block; MarshalJSON emits only the fields of its type.
type Block struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Fill     string `json:"fill"`
	Ink      string `json:"ink"`
	Pad      int    `json:"pad"`
	Gap      int    `json:"gap"`
	TypeRole string `json:"typeRole"`
	Weight   int    `json:"weight"`
	Align    string `json:"align"`

	Brand      string  `json:"brand"`
	Links      []*Link `json:"links"`
	Eyebrow    string  `json:"eyebrow"`
	Headline   string  `json:"headline"`
	Body       string  `json:"body"`
	BodyRole   string  `json:"bodyRole"`
	CTA        *Link   `json:"cta"`
	Secondary  *Link   `json:"secondary"`
	Heading    string  `json:"heading"`
	Items      []*Item `json:"items"`
	Text       string  `json:"text"`
	Cite       string  `json:"cite"`
	Button     *Link   `json:"button"`
	ButtonFill string  `json:"buttonFill"`
	ButtonInk  string  `json:"buttonInk"`
	Note       string  `json:"note"`
}

// BlockTypes in palette order.
var BlockTypes = []string{"nav", "hero", "features", "quote", "cta", "footer"}

type blockBase struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Fill     string `json:"fill"`
	Ink      string `json:"ink"`
	Pad      int    `json:"pad"`
	Gap      int    `json:"gap"`
	TypeRole string `json:"typeRole"`
	Weight   int    `json:"weight"`
	Align    string `json:"align"`
}

func links(l []*Link) []*Link {
	if l == nil {
		return []*Link{}
	}
	return l
}

func (b *Block) MarshalJSON() ([]byte, error) {
	base := blockBase{b.ID, b.Type, b.Fill, b.Ink, b.Pad, b.Gap, b.TypeRole, b.Weight, b.Align}
	switch b.Type {
	case "nav":
		return json.Marshal(struct {
			blockBase
			Brand string  `json:"brand"`
			Links []*Link `json:"links"`
		}{base, b.Brand, links(b.Links)})
	case "hero":
		cta := b.CTA
		if cta == nil {
			cta = &Link{}
		}
		return json.Marshal(struct {
			blockBase
			Eyebrow   string `json:"eyebrow"`
			Headline  string `json:"headline"`
			Body      string `json:"body"`
			BodyRole  string `json:"bodyRole"`
			CTA       *Link  `json:"cta"`
			Secondary *Link  `json:"secondary"`
		}{base, b.Eyebrow, b.Headline, b.Body, b.BodyRole, cta, b.Secondary})
	case "features":
		items := b.Items
		if items == nil {
			items = []*Item{}
		}
		return json.Marshal(struct {
			blockBase
			Heading string  `json:"heading"`
			Items   []*Item `json:"items"`
		}{base, b.Heading, items})
	case "quote":
		return json.Marshal(struct {
			blockBase
			Text string `json:"text"`
			Cite string `json:"cite"`
		}{base, b.Text, b.Cite})
	case "cta":
		btn := b.Button
		if btn == nil {
			btn = &Link{}
		}
		return json.Marshal(struct {
			blockBase
			Headline   string `json:"headline"`
			Body       string `json:"body"`
			Button     *Link  `json:"button"`
			ButtonFill string `json:"buttonFill"`
			ButtonInk  string `json:"buttonInk"`
		}{base, b.Headline, b.Body, btn, b.ButtonFill, b.ButtonInk})
	case "footer":
		return json.Marshal(struct {
			blockBase
			Brand string  `json:"brand"`
			Note  string  `json:"note"`
			Links []*Link `json:"links"`
		}{base, b.Brand, b.Note, links(b.Links)})
	}
	return json.Marshal(base)
}

type Draw struct {
	Shapes []*Shape `json:"shapes"`
	View   DrawView `json:"view"`
}

type DrawView struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// CanvasState holds the shared infinite-canvas state (§8.2).
type CanvasState struct {
	View CanvasView `json:"view"`
}

// CanvasView is the persisted viewport; Zoom 0 means "unset: fit everything on open".
type CanvasView struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// ShapeTypes accepted by add_shape.
var ShapeTypes = []string{"rect", "ellipse", "line", "arrow", "text", "pen"}

// Shape is any draw shape; MarshalJSON emits only the fields of its type.
type Shape struct {
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	X      float64      `json:"x"`
	Y      float64      `json:"y"`
	W      float64      `json:"w"`
	H      float64      `json:"h"`
	X1     float64      `json:"x1"`
	Y1     float64      `json:"y1"`
	X2     float64      `json:"x2"`
	Y2     float64      `json:"y2"`
	Stroke string       `json:"stroke"`
	Fill   string       `json:"fill"`
	SW     float64      `json:"sw"`
	Text   string       `json:"text"`
	Role   string       `json:"role"`
	Ink    string       `json:"ink"`
	Points [][2]float64 `json:"points"`
}

func (s *Shape) MarshalJSON() ([]byte, error) {
	switch s.Type {
	case "rect", "ellipse":
		return json.Marshal(struct {
			ID     string  `json:"id"`
			Type   string  `json:"type"`
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			W      float64 `json:"w"`
			H      float64 `json:"h"`
			Stroke string  `json:"stroke"`
			Fill   string  `json:"fill"`
			SW     float64 `json:"sw"`
		}{s.ID, s.Type, s.X, s.Y, s.W, s.H, s.Stroke, s.Fill, s.SW})
	case "line", "arrow":
		return json.Marshal(struct {
			ID     string  `json:"id"`
			Type   string  `json:"type"`
			X1     float64 `json:"x1"`
			Y1     float64 `json:"y1"`
			X2     float64 `json:"x2"`
			Y2     float64 `json:"y2"`
			Stroke string  `json:"stroke"`
			SW     float64 `json:"sw"`
		}{s.ID, s.Type, s.X1, s.Y1, s.X2, s.Y2, s.Stroke, s.SW})
	case "text":
		return json.Marshal(struct {
			ID   string  `json:"id"`
			Type string  `json:"type"`
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			Text string  `json:"text"`
			Role string  `json:"role"`
			Ink  string  `json:"ink"`
		}{s.ID, s.Type, s.X, s.Y, s.Text, s.Role, s.Ink})
	case "pen":
		pts := s.Points
		if pts == nil {
			pts = [][2]float64{}
		}
		return json.Marshal(struct {
			ID     string       `json:"id"`
			Type   string       `json:"type"`
			Points [][2]float64 `json:"points"`
			Stroke string       `json:"stroke"`
			SW     float64      `json:"sw"`
		}{s.ID, s.Type, pts, s.Stroke, s.SW})
	}
	return json.Marshal(struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}{s.ID, s.Type})
}

type Doc struct {
	Markdown string `json:"markdown"`
}

type PinTarget struct {
	Type    string `json:"type"`
	NodeID  string `json:"nodeId,omitempty"`
	BlockID string `json:"blockId,omitempty"`
}

type Pin struct {
	ID        string    `json:"id"`
	N         int       `json:"n"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	Resolved  bool      `json:"resolved"`
	Target    PinTarget `json:"target"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	CreatedAt int64     `json:"createdAt"`
}

// ---- shared view state (§6.2) ----

type ViewModes struct {
	Web   string `json:"web"`
	Story string `json:"story"`
}

type CurrentNodes struct {
	Web    Ref `json:"web"`
	Mobile Ref `json:"mobile"`
}

type Selection struct {
	Kind      string `json:"kind"`
	NodeID    string `json:"nodeId,omitempty"`
	BlockID   string `json:"blockId,omitempty"`
	ChildID   string `json:"childId,omitempty"`
	HotspotID string `json:"hotspotId,omitempty"`
	EdgeID    string `json:"edgeId,omitempty"`
	ShapeID   string `json:"shapeId,omitempty"`
	PinID     string `json:"pinId,omitempty"`
}

type SharedView struct {
	Tab           string       `json:"tab"`
	View          ViewModes    `json:"view"`
	Breakpoint    string       `json:"breakpoint"`
	CurrentNodeID CurrentNodes `json:"currentNodeId"`
	Selection     *Selection   `json:"selection"`
	Preview       bool         `json:"preview"`
	ShowPins      bool         `json:"showPins"`
	ShowGrid      bool         `json:"showGrid"`
	ShowSpecs     bool         `json:"showSpecs"`
	Entered       Ref          `json:"entered"` // node being edited in place on the canvas, or null
}

// DefaultView is the view state for a fresh project.
func DefaultView() SharedView {
	return SharedView{
		Tab:           "web",
		View:          ViewModes{Web: "page", Story: "page"},
		Breakpoint:    "desktop",
		CurrentNodeID: CurrentNodes{Web: "pg_home", Mobile: "sc_welcome"},
		ShowPins:      true,
	}
}

// Clone deep-copies the project.
func (p *Project) Clone() *Project {
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	var q Project
	if err := json.Unmarshal(b, &q); err != nil {
		panic(err)
	}
	q.normalize()
	return &q
}

// normalize replaces nil slices/maps so JSON stays in shape.
func (p *Project) normalize() {
	if p.Flows == nil {
		p.Flows = []*Flow{}
	}
	if p.Nodes == nil {
		p.Nodes = []*Node{}
	}
	if p.Edges == nil {
		p.Edges = []*Edge{}
	}
	if p.Pins == nil {
		p.Pins = []*Pin{}
	}
	if p.Draw.Shapes == nil {
		p.Draw.Shapes = []*Shape{}
	}
	if p.Tokens.Colors == nil {
		p.Tokens.Colors = map[string]string{}
	}
	if p.Tokens.Type == nil {
		p.Tokens.Type = map[string]TypeSpec{}
	}
	if p.Draw.View.Zoom == 0 {
		p.Draw.View.Zoom = 1
	}
	for _, n := range p.Nodes {
		if n.Kind == "page" && n.Blocks == nil {
			n.Blocks = []*Block{}
		}
		if n.Kind == "screen" && n.Hotspots == nil {
			n.Hotspots = []*Hotspot{}
		}
	}
}

func (v SharedView) clone() SharedView {
	if v.Selection != nil {
		s := *v.Selection
		v.Selection = &s
	}
	return v
}

// ---- lookups ----

func (p *Project) Node(id string) *Node {
	for _, n := range p.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (p *Project) Flow(id string) *Flow {
	for _, f := range p.Flows {
		if f.ID == id {
			return f
		}
	}
	return nil
}

func (p *Project) Edge(id string) *Edge {
	for _, e := range p.Edges {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func (p *Project) Pin(id string) *Pin {
	for _, x := range p.Pins {
		if x.ID == id {
			return x
		}
	}
	return nil
}

func (p *Project) Shape(id string) *Shape {
	for _, s := range p.Draw.Shapes {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (n *Node) Block(id string) *Block {
	for _, b := range n.Blocks {
		if b.ID == id {
			return b
		}
	}
	return nil
}

func (n *Node) Hotspot(id string) *Hotspot {
	for _, h := range n.Hotspots {
		if h.ID == id {
			return h
		}
	}
	return nil
}

// FlowsOf returns the flows of a channel in project order.
func (p *Project) FlowsOf(ch string) []*Flow {
	var out []*Flow
	for _, f := range p.Flows {
		if f.Channel == ch {
			out = append(out, f)
		}
	}
	return out
}

// NodesOfFlow returns the flow's nodes in project order.
func (p *Project) NodesOfFlow(flowID string) []*Node {
	var out []*Node
	for _, n := range p.Nodes {
		if n.FlowID == flowID {
			out = append(out, n)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
