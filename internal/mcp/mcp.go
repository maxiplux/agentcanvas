// Package mcp exposes the Store as intent tools over MCP: streamable HTTP at
// /mcp (stateless), stdio, and the attach proxy (§13). Handlers close over the
// Store; MCP session state stays empty. Nothing here reads or emits
// vendor-specific headers, prompts, or config formats.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	// SPEC: jsonschema-go is the go-sdk's own schema package (Tool.InputSchema);
	// importing it directly lets tool schemas carry enums. No new module enters the build.
	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"agentcanvas/internal/export"
	"agentcanvas/internal/skills"
	"agentcanvas/internal/studio"
)

// Instructions is the initialize instructions text (§13.2).
const Instructions = "Drive the AgentCanvas studio only through these tools. Call `get_canvas` before editing. Read `skills_list` before inventing a workflow. Trust each tool result's `summary` and `changedIds` for what changed; do not assume."

// GuideURI is the MCP resource carrying the guide markdown.
const GuideURI = "agentcanvas://guide"

// Tool groups in catalog order (§13.3).
var Groups = []string{"Read", "Navigate", "History", "Edit", "Output", "Packages"}

// Options configure the MCP server.
type Options struct {
	Store   *studio.Store
	Skills  *skills.Store
	Version string
}

// ToolDoc is one registered tool, used for the generated guide.
type ToolDoc struct {
	Group       string
	Name        string
	Description string
}

// Server wraps the go-sdk server.
type Server struct {
	o     Options
	srv   *sdk.Server
	tools []ToolDoc
	http  http.Handler
}

// New builds the MCP server with every tool registered.
func New(o Options) *Server {
	if o.Version == "" {
		o.Version = "dev"
	}
	s := &Server{o: o}
	s.srv = sdk.NewServer(&sdk.Implementation{Name: "agentcanvas", Title: "AgentCanvas", Version: o.Version},
		&sdk.ServerOptions{Instructions: Instructions})
	s.registerTools()
	s.srv.AddResource(&sdk.Resource{
		URI:         GuideURI,
		Name:        "guide",
		Title:       "AgentCanvas guide",
		Description: "What the studio is, the tool catalog, and where to start (call get_canvas first).",
		MIMEType:    "text/markdown",
	}, func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: GuideURI, MIMEType: "text/markdown", Text: s.Guide()}}}, nil
	})
	// SPEC: JSONResponse makes every POST answer plain application/json, so a raw
	// curl client (only Content-Type and Accept headers) reads results directly.
	s.http = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.srv },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return s
}

// SDK returns the underlying go-sdk server (tests, in-memory transports).
func (s *Server) SDK() *sdk.Server { return s.srv }

// Tools lists the registered tools in catalog order.
func (s *Server) Tools() []ToolDoc { return append([]ToolDoc(nil), s.tools...) }

// Handler serves streamable HTTP at /mcp.
func (s *Server) Handler() http.Handler { return s.http }

// RunStdio serves MCP on stdin/stdout until ctx ends or stdin closes.
func (s *Server) RunStdio(ctx context.Context) error {
	return s.srv.Run(ctx, &sdk.StdioTransport{})
}

// Guide is the markdown served at GET /guide and agentcanvas://guide (§13.2).
func (s *Server) Guide() string {
	var b strings.Builder
	b.WriteString("# AgentCanvas\n\n")
	b.WriteString("AgentCanvas is a local design studio for web pages, mobile story flows, connected storyboards, an SVG drawing, a `DESIGN.md`, and numbered review pins. ")
	b.WriteString("One process serves one project; the designer's browser and every connected agent edit the same canvas and see each other's changes live.\n\n")
	b.WriteString("**Call `get_canvas` first.** It is the screen substitute: the shared view, tokens, flows in storyboard order, nodes with block and child ids, edges, doc headings, pins, failing checks, and installed skills. ")
	b.WriteString("Read `skills_list` before inventing a workflow. Every edit returns `summary` and `changedIds`; trust them for what changed. ")
	b.WriteString("A refused action leaves the project unchanged and its text names the rule and what to call next.\n\n")
	b.WriteString("## Tools\n")
	for _, g := range Groups {
		fmt.Fprintf(&b, "\n### %s\n\n", g)
		for _, t := range s.tools {
			if t.Group == g {
				fmt.Fprintf(&b, "- `%s` — %s\n", t.Name, t.Description)
			}
		}
	}
	return b.String()
}

// ---- helpers ----

func marshal(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return strings.TrimRight(buf.String(), "\n")
}

func text(s string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: s}}}
}

func refusal(err error) *sdk.CallToolResult {
	var ae *studio.ActionError
	msg := ""
	if errors.As(err, &ae) {
		msg = ae.Error()
	} else {
		msg = "Refused: " + strings.TrimSuffix(err.Error(), ".") + "."
	}
	r := text(msg)
	r.IsError = true
	return r
}

func refused(msg, next string) *sdk.CallToolResult {
	return refusal(&studio.ActionError{Message: msg, Next: next})
}

// apply sends one action to the Store and renders the §13.4 result.
func (s *Server) apply(typ string, payload any, tab string) *sdk.CallToolResult {
	b, err := json.Marshal(payload)
	if err != nil {
		return refusal(err)
	}
	res, err := s.o.Store.Apply(studio.Action{Type: typ, Payload: b, Tab: tab}, "mcp")
	if err != nil {
		return refusal(err)
	}
	return text(marshal(res))
}

// add registers a typed tool with an inferred schema plus enum constraints.
func add[In any](s *Server, group, name, desc string, enums map[string][]string, readOnly bool, h func(context.Context, In) *sdk.CallToolResult) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("tool %s: %v", name, err))
	}
	for path, vals := range enums {
		applyEnum(schema, strings.Split(path, "."), vals)
	}
	t := &sdk.Tool{Name: name, Description: desc, InputSchema: schema}
	if readOnly {
		t.Annotations = &sdk.ToolAnnotations{ReadOnlyHint: true}
	}
	sdk.AddTool(s.srv, t, func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		return h(ctx, in), nil, nil
	})
	s.tools = append(s.tools, ToolDoc{Group: group, Name: name, Description: desc})
}

func applyEnum(sch *jsonschema.Schema, path []string, vals []string) {
	if sch == nil || len(path) == 0 {
		return
	}
	p := sch.Properties[path[0]]
	if p == nil {
		panic("enum path not found: " + strings.Join(path, "."))
	}
	if len(path) > 1 {
		applyEnum(p, path[1:], vals)
		return
	}
	var e []any
	for _, v := range vals {
		e = append(e, v)
	}
	for _, t := range p.Types {
		if t == "null" {
			e = append(e, nil)
		}
	}
	p.Enum = e
}

var (
	enumTab        = []string{"web", "story", "draw", "doc"}
	enumColor      = []string{"paper", "ink", "muted", "accent", "wash", "line"}
	enumChannel    = []string{"web", "mobile"}
	enumArchetype  = []string{"title", "photo", "quote", "cta", "split"}
	enumBlock      = []string{"nav", "hero", "features", "quote", "cta", "footer"}
	enumEdgeKind   = []string{"primary", "alt", "back", "cross"}
	enumPinKind    = []string{"issue", "suggestion", "question"}
	enumShape      = []string{"rect", "ellipse", "line", "arrow", "text", "pen"}
	enumExportKind = []string{"page", "web-flows", "mobile-flow", "draw-svg", "design-md"}
	enumSelection  = []string{"node", "block", "hotspot", "edge", "shape", "pin"}
)

// ---- input types (field names match the §12.3 action payloads) ----

type emptyIn struct{}

type getCanvasIn struct {
	Full bool `json:"full,omitempty" jsonschema:"also return the complete Project JSON"`
}

type nodeIn struct {
	NodeID string `json:"nodeId" jsonschema:"node id (page like pg_home or screen like sc_welcome)"`
}

type setTabIn struct {
	Tab  string `json:"tab" jsonschema:"web, story, draw, or doc"`
	View string `json:"view,omitempty" jsonschema:"page or board (storyboard); web and story tabs only"`
}

type setBreakpointIn struct {
	Breakpoint string `json:"breakpoint" jsonschema:"desktop (1280), tablet (768), or phone (390)"`
}

type selectionIn struct {
	Kind      string `json:"kind" jsonschema:"node, block, hotspot, edge, shape, or pin"`
	NodeID    string `json:"nodeId,omitempty" jsonschema:"for node, block, hotspot"`
	BlockID   string `json:"blockId,omitempty" jsonschema:"for block"`
	ChildID   string `json:"childId,omitempty" jsonschema:"optional child inside the block (link or feature item id)"`
	HotspotID string `json:"hotspotId,omitempty" jsonschema:"for hotspot"`
	EdgeID    string `json:"edgeId,omitempty" jsonschema:"for edge"`
	ShapeID   string `json:"shapeId,omitempty" jsonschema:"for shape"`
	PinID     string `json:"pinId,omitempty" jsonschema:"for pin"`
}

type setSelectionIn struct {
	Selection *selectionIn `json:"selection" jsonschema:"what to select, or null to clear the selection"`
}

type setTogglesIn struct {
	ShowPins  *bool `json:"showPins,omitempty" jsonschema:"show review pins while designing"`
	ShowGrid  *bool `json:"showGrid,omitempty" jsonschema:"12-column overlay on the web page frame"`
	ShowSpecs *bool `json:"showSpecs,omitempty" jsonschema:"red-line px labels on the selection"`
}

type previewIn struct {
	On bool `json:"on" jsonschema:"true enters Preview/Play, false exits"`
}

type tabIn struct {
	Tab string `json:"tab,omitempty" jsonschema:"history to use: web, story, draw, or doc (default: the shared current tab)"`
}

type resetIn struct {
	Confirm bool `json:"confirm" jsonschema:"must be true"`
}

type typePatch struct {
	Size   *int `json:"size,omitempty" jsonschema:"font size px"`
	LH     *int `json:"lh,omitempty" jsonschema:"line-height px"`
	Weight *int `json:"weight,omitempty" jsonschema:"400, 500, or 600"`
}

type updateTokensIn struct {
	Colors map[string]string    `json:"colors,omitempty" jsonschema:"token key (paper ink muted accent wash line) to hex like #D8432A"`
	Type   map[string]typePatch `json:"type,omitempty" jsonschema:"type key (xs sm md lg xl display) to {size, lh, weight}"`
	Space  []int                `json:"space,omitempty" jsonschema:"always refused: the space scale [0 4 8 16 24 32 48 64] is fixed"`
	Tab    string               `json:"tab,omitempty" jsonschema:"history that records the edit: web (default) or story"`
}

type tokensPayload struct {
	Colors map[string]string    `json:"colors,omitempty"`
	Type   map[string]typePatch `json:"type,omitempty"`
	Space  []int                `json:"space,omitempty"`
}

type upsertFlowIn struct {
	ID      string  `json:"id,omitempty" jsonschema:"existing flow id to update; omit to create"`
	Channel string  `json:"channel,omitempty" jsonschema:"web or mobile (required to create)"`
	Name    *string `json:"name,omitempty" jsonschema:"flow name"`
	StartID *string `json:"startId,omitempty" jsonschema:"start node; must belong to this flow"`
}

type flowIn struct {
	FlowID string `json:"flowId" jsonschema:"flow id"`
}

type upsertNodeIn struct {
	ID        string  `json:"id,omitempty" jsonschema:"existing node id to update; omit to create"`
	FlowID    string  `json:"flowId,omitempty" jsonschema:"flow to create in, or to move an existing node into (same channel)"`
	Name      *string `json:"name,omitempty" jsonschema:"page or screen name"`
	Archetype string  `json:"archetype,omitempty" jsonschema:"screens only: title, photo, quote, cta, or split (default title)"`
	AfterID   string  `json:"afterId,omitempty" jsonschema:"create only: insert after this node"`
}

type reorderNodesIn struct {
	FlowID  string   `json:"flowId" jsonschema:"flow id"`
	NodeIDs []string `json:"nodeIds" jsonschema:"every node id of the flow, in the new order"`
}

type insertBlockIn struct {
	NodeID       string `json:"nodeId" jsonschema:"web page id"`
	Type         string `json:"type" jsonschema:"nav, hero, features, quote, cta, or footer"`
	AfterBlockID string `json:"afterBlockId,omitempty" jsonschema:"insert after this block (default: at the end)"`
}

type updateBlockIn struct {
	NodeID  string         `json:"nodeId" jsonschema:"web page id"`
	BlockID string         `json:"blockId" jsonschema:"block id"`
	ChildID string         `json:"childId,omitempty" jsonschema:"patch a child instead: a link ({label}) or a feature item ({title, body})"`
	Patch   map[string]any `json:"patch" jsonschema:"fields to set. Design: fill, ink (color keys), pad, gap (space index 0-7), typeRole (xs..display), weight (400/500/600), align (left/center). Content by type: nav brand; hero eyebrow, headline, body, bodyRole, cta {label}, secondary {label} or null; features heading; quote text, cite; cta headline, body, button {label}, buttonFill, buttonInk; footer brand, note"`
}

type upsertChildIn struct {
	NodeID  string         `json:"nodeId" jsonschema:"web page id"`
	BlockID string         `json:"blockId" jsonschema:"nav, footer, or features block id"`
	ChildID string         `json:"childId,omitempty" jsonschema:"existing child to update; omit to append a new one"`
	Fields  map[string]any `json:"fields,omitempty" jsonschema:"links: {label}; feature items: {title, body}"`
}

type childIn struct {
	NodeID  string `json:"nodeId" jsonschema:"web page id"`
	BlockID string `json:"blockId" jsonschema:"block id"`
	ChildID string `json:"childId" jsonschema:"link or feature item id"`
}

type reorderBlocksIn struct {
	NodeID   string   `json:"nodeId" jsonschema:"web page id"`
	BlockIDs []string `json:"blockIds" jsonschema:"every block id of the page, in the new order"`
}

type blockIn struct {
	NodeID  string `json:"nodeId" jsonschema:"web page id"`
	BlockID string `json:"blockId" jsonschema:"block id"`
}

type updateScreenIn struct {
	NodeID string         `json:"nodeId" jsonschema:"mobile screen id"`
	Patch  map[string]any `json:"patch" jsonschema:"fields to set: name, title, body, archetype, layout (center/bottom/split), fill, ink, typeRole, weight, align, pad, gap, cta ({label, size sm/md/lg, fill, ink} or null to remove)"`
}

type upsertHotspotIn struct {
	NodeID    string   `json:"nodeId" jsonschema:"mobile screen id"`
	HotspotID string   `json:"hotspotId,omitempty" jsonschema:"existing hotspot to update; omit to create"`
	Label     *string  `json:"label,omitempty" jsonschema:"hotspot label (also the edge label when it has a target)"`
	X         *float64 `json:"x,omitempty" jsonschema:"left px within the 390-wide screen"`
	Y         *float64 `json:"y,omitempty" jsonschema:"top px within the 844-tall screen"`
	W         *float64 `json:"w,omitempty" jsonschema:"width px (44 or more passes the tap check)"`
	H         *float64 `json:"h,omitempty" jsonschema:"height px (44 or more passes the tap check)"`
}

type hotspotIn struct {
	NodeID    string `json:"nodeId" jsonschema:"mobile screen id"`
	HotspotID string `json:"hotspotId" jsonschema:"hotspot id"`
}

type linkOwnerIn struct {
	BlockID   string `json:"blockId,omitempty" jsonschema:"page block that owns the link"`
	ChildID   string `json:"childId,omitempty" jsonschema:"with blockId: a nav or footer link id"`
	Field     string `json:"field,omitempty" jsonschema:"with blockId: cta or secondary (hero), button (cta band)"`
	CTA       bool   `json:"cta,omitempty" jsonschema:"true for a mobile screen's CTA"`
	HotspotID string `json:"hotspotId,omitempty" jsonschema:"a mobile screen hotspot id"`
}

type setLinkTargetIn struct {
	NodeID string      `json:"nodeId" jsonschema:"node that owns the link"`
	Owner  linkOwnerIn `json:"owner" jsonschema:"which link: {blockId, childId}, {blockId, field}, {cta: true}, or {hotspotId}"`
	ToID   *string     `json:"toId" jsonschema:"target node id in the same channel, or null to clear"`
}

type upsertEdgeIn struct {
	ID     string  `json:"id,omitempty" jsonschema:"existing edge id to update; omit to create"`
	FromID string  `json:"fromId,omitempty" jsonschema:"source node (required to create)"`
	ToID   string  `json:"toId,omitempty" jsonschema:"target node in the same channel (required to create)"`
	Label  *string `json:"label,omitempty" jsonschema:"edge label (default Link); different labels make branches"`
	Kind   string  `json:"kind,omitempty" jsonschema:"primary, alt, or back for same-flow edges; cross-flow edges are always cross"`
}

type edgeIn struct {
	EdgeID string `json:"edgeId" jsonschema:"edge id"`
}

type shapeIn struct {
	Type   string       `json:"type" jsonschema:"rect, ellipse, line, arrow, text, or pen"`
	X      *float64     `json:"x,omitempty" jsonschema:"rect/ellipse/text: left (world px)"`
	Y      *float64     `json:"y,omitempty" jsonschema:"rect/ellipse/text: top (world px)"`
	W      *float64     `json:"w,omitempty" jsonschema:"rect/ellipse: width"`
	H      *float64     `json:"h,omitempty" jsonschema:"rect/ellipse: height"`
	X1     *float64     `json:"x1,omitempty" jsonschema:"line/arrow: start x"`
	Y1     *float64     `json:"y1,omitempty" jsonschema:"line/arrow: start y"`
	X2     *float64     `json:"x2,omitempty" jsonschema:"line/arrow: end x"`
	Y2     *float64     `json:"y2,omitempty" jsonschema:"line/arrow: end y"`
	Stroke string       `json:"stroke,omitempty" jsonschema:"stroke color key (default ink)"`
	Fill   string       `json:"fill,omitempty" jsonschema:"rect/ellipse fill color key or none (default none)"`
	SW     *float64     `json:"sw,omitempty" jsonschema:"stroke width 1, 1.5, 2, or 3 (default 1.5)"`
	Text   string       `json:"text,omitempty" jsonschema:"text shapes: the text"`
	Role   string       `json:"role,omitempty" jsonschema:"text shapes: type role (default md)"`
	Ink    string       `json:"ink,omitempty" jsonschema:"text shapes: color key (default ink)"`
	Points [][2]float64 `json:"points,omitempty" jsonschema:"pen: [[x,y],...] with at least 2 points"`
}

type addShapeIn struct {
	Shape shapeIn `json:"shape" jsonschema:"the shape without an id"`
}

type updateShapeIn struct {
	ShapeID string         `json:"shapeId" jsonschema:"shape id"`
	Patch   map[string]any `json:"patch" jsonschema:"fields valid for the shape type (x, y, w, h, x1, y1, x2, y2, stroke, fill, sw, text, role, ink, points)"`
}

type shapeIDIn struct {
	ShapeID string `json:"shapeId" jsonschema:"shape id"`
}

type setDocIn struct {
	Markdown string `json:"markdown" jsonschema:"the complete DESIGN.md markdown"`
}

type pinTargetIn struct {
	Type    string `json:"type" jsonschema:"node (a page or screen), board (a storyboard thumbnail), or draw"`
	NodeID  string `json:"nodeId,omitempty" jsonschema:"node and board targets"`
	BlockID string `json:"blockId,omitempty" jsonschema:"optional block of a page"`
}

type addPinIn struct {
	Kind   string      `json:"kind" jsonschema:"issue, suggestion, or question"`
	Body   string      `json:"body" jsonschema:"the note (must not be empty)"`
	Target pinTargetIn `json:"target" jsonschema:"where the pin sits"`
	X      float64     `json:"x" jsonschema:"fraction 0..1 of the block, frame, or thumbnail; world px for draw"`
	Y      float64     `json:"y" jsonschema:"fraction 0..1 of the block, frame, or thumbnail; world px for draw"`
}

type updatePinIn struct {
	PinID    string   `json:"pinId" jsonschema:"pin id"`
	Kind     *string  `json:"kind,omitempty" jsonschema:"issue, suggestion, or question"`
	Body     *string  `json:"body,omitempty" jsonschema:"new note (not empty)"`
	Resolved *bool    `json:"resolved,omitempty" jsonschema:"true resolves, false reopens"`
	X        *float64 `json:"x,omitempty" jsonschema:"new x"`
	Y        *float64 `json:"y,omitempty" jsonschema:"new y"`
}

type pinIn struct {
	PinID string `json:"pinId" jsonschema:"pin id"`
}

type exportIn struct {
	Kind   string `json:"kind" jsonschema:"page, web-flows, mobile-flow, draw-svg, or design-md"`
	NodeID string `json:"nodeId,omitempty" jsonschema:"page: the web page (default: the current web page)"`
	FlowID string `json:"flowId,omitempty" jsonschema:"mobile-flow: the flow (default: the current screen's flow)"`
	Write  bool   `json:"write,omitempty" jsonschema:"also save into the project: DESIGN.md at the root, others under .agentcanvas/exports/"`
}

type skillsInstallIn struct {
	Path  string `json:"path" jsonschema:"folder with SKILL.md and skill.json (relative paths resolve against the project directory)"`
	Force bool   `json:"force,omitempty" jsonschema:"replace an installed skill of the same name"`
}

type skillsRemoveIn struct {
	Name string `json:"name" jsonschema:"installed skill name"`
}

// ---- registration ----

func (s *Server) registerTools() {
	st := s.o.Store

	// Read
	add(s, "Read", "get_canvas",
		"Read the whole studio as JSON: the screen substitute. Returns rev, the shared view (tab, view, breakpoint, currentNodeId, selection, toggles), history flags, tokens, flows with node ids in storyboard lane order, nodes (pages with blocks, child ids, labels and toIds; screens with archetype, title, cta, hotspots), edges, doc headings, pins, failing checks, openIssues, and installed skills. Input: full=true adds the complete Project. Call before editing; never fails.",
		nil, true, func(_ context.Context, in getCanvasIn) *sdk.CallToolResult { return text(marshal(st.Canvas(in.Full))) })
	add(s, "Read", "get_node",
		"Read one page or screen in full, with its inbound and outbound edges and its pins. Input: nodeId. Fails for an unknown id (call get_canvas for ids).",
		nil, true, func(_ context.Context, in nodeIn) *sdk.CallToolResult {
			d, err := st.NodeDetail(in.NodeID)
			if err != nil {
				return refusal(err)
			}
			return text(marshal(d))
		})
	add(s, "Read", "get_doc",
		"Read the DESIGN.md markdown of the Doc tab. No input; never fails.",
		nil, true, func(_ context.Context, _ emptyIn) *sdk.CallToolResult { return text(st.Doc()) })
	add(s, "Read", "skills_list",
		"List installed skills (the built-in agentcanvas skill first) with name, version, description, builtin, and the SKILL.md body. Read it before inventing a workflow. No input; never fails.",
		nil, true, func(_ context.Context, _ emptyIn) *sdk.CallToolResult { return text(marshal(s.o.Skills.ListFull())) })

	// Navigate
	add(s, "Navigate", "set_tab",
		"Switch the shared tab (web, story, draw, doc) that every open browser shows, optionally with view page or board (storyboard) for web and story. Fails on an unknown tab, or a view on draw/doc.",
		map[string][]string{"tab": enumTab, "view": {"page", "board"}}, false,
		func(_ context.Context, in setTabIn) *sdk.CallToolResult { return s.apply("set_tab", in, "") })
	add(s, "Navigate", "set_breakpoint",
		"Set the shared web breakpoint: desktop (1280), tablet (768), or phone (390). Checks (contrast, tap targets) are computed at this breakpoint. Fails on an unknown breakpoint.",
		map[string][]string{"breakpoint": {"desktop", "tablet", "phone"}}, false,
		func(_ context.Context, in setBreakpointIn) *sdk.CallToolResult {
			return s.apply("set_breakpoint", in, "")
		})
	add(s, "Navigate", "set_selection",
		"Set the shared selection: {kind: node|block|hotspot|edge|shape|pin, ...ids} or null. Selecting a node, block, or hotspot makes its node current and switches to its tab; selecting a pin navigates to it. Fails on unknown ids.",
		map[string][]string{"selection.kind": enumSelection}, false,
		func(_ context.Context, in setSelectionIn) *sdk.CallToolResult {
			return s.apply("set_selection", in, "")
		})
	add(s, "Navigate", "set_toggles",
		"Turn shared overlays on or off: showPins, showGrid (12-column overlay), showSpecs (px labels). Send at least one; fails when none is given.",
		nil, false, func(_ context.Context, in setTogglesIn) *sdk.CallToolResult { return s.apply("set_toggles", in, "") })
	add(s, "Navigate", "preview",
		"Enter (on=true) or exit (on=false) Preview/Play in every open browser: chrome hidden, links follow edges. Never fails.",
		nil, false, func(_ context.Context, in previewIn) *sdk.CallToolResult { return s.apply("set_preview", in, "") })

	// History
	add(s, "History", "undo",
		"Undo the last change in one tab history (web, story, draw, doc), shared with the designer; default is the shared current tab. Fails when that history is empty.",
		map[string][]string{"tab": enumTab}, false,
		func(_ context.Context, in tabIn) *sdk.CallToolResult { return s.apply("undo", in, "") })
	add(s, "History", "redo",
		"Redo the last undone change in one tab history; default is the shared current tab. Fails when nothing was undone.",
		map[string][]string{"tab": enumTab}, false,
		func(_ context.Context, in tabIn) *sdk.CallToolResult { return s.apply("redo", in, "") })
	add(s, "History", "reset_project",
		"Replace the whole project with the starter seed and clear all histories. Input: confirm=true; refused otherwise.",
		nil, false, func(_ context.Context, in resetIn) *sdk.CallToolResult {
			if !in.Confirm {
				return refused("reset_project replaces the whole project and needs confirm: true", "call reset_project with {\"confirm\": true} only if the user asked to start over")
			}
			return s.apply("reset_project", struct{}{}, "")
		})

	// Edit
	add(s, "Edit", "update_tokens",
		"Change shared tokens: colors (key to hex) and type (key to size, lh, weight). Every block and screen using the token updates. Input tab picks the history (web default, or story). Fails on unknown keys or bad values; space is refused because the scale [0 4 8 16 24 32 48 64] is fixed.",
		map[string][]string{"tab": {"web", "story"}}, false,
		func(_ context.Context, in updateTokensIn) *sdk.CallToolResult {
			tab := in.Tab
			if tab == "" {
				tab = "web"
			}
			return s.apply("update_tokens", tokensPayload{Colors: in.Colors, Type: in.Type, Space: in.Space}, tab)
		})
	add(s, "Edit", "upsert_flow",
		"Create a flow (omit id; channel required; it gets one empty page or screen as its start) or rename it / set its start (id given). Fails if startId is not in the flow or the channel changes.",
		map[string][]string{"channel": enumChannel}, false,
		func(_ context.Context, in upsertFlowIn) *sdk.CallToolResult { return s.apply("upsert_flow", in, "") })
	add(s, "Edit", "delete_flow",
		"Delete a flow and its nodes, their edges, pins, and every link target pointing at them. Fails on the last flow of a channel (a channel keeps at least one flow).",
		nil, false, func(_ context.Context, in flowIn) *sdk.CallToolResult { return s.apply("delete_flow", in, "") })
	add(s, "Edit", "upsert_node",
		"Create a page (web flow) or screen (mobile flow, from an archetype) in flowId, or rename / change archetype / move an existing node (id given) to another flow of the same channel. Fails on unknown ids or a cross-channel move.",
		map[string][]string{"archetype": enumArchetype}, false,
		func(_ context.Context, in upsertNodeIn) *sdk.CallToolResult { return s.apply("upsert_node", in, "") })
	add(s, "Edit", "reorder_nodes",
		"Reorder the nodes of a flow (node order only; edges and storyboard BFS order are unchanged). Input nodeIds must list every node of the flow exactly once.",
		nil, false, func(_ context.Context, in reorderNodesIn) *sdk.CallToolResult {
			return s.apply("reorder_nodes", in, "")
		})
	add(s, "Edit", "delete_node",
		"Delete a page or screen with its edges (both directions), its pins, and every link target pointing at it; the flow start moves to the next node. Fails on an unknown id.",
		nil, false, func(_ context.Context, in nodeIn) *sdk.CallToolResult { return s.apply("delete_node", in, "") })
	add(s, "Edit", "insert_block",
		"Insert a block (nav, hero, features, quote, cta, footer) with defaults into a web page, after afterBlockId or at the end. Fails if nodeId is a mobile screen.",
		map[string][]string{"type": enumBlock}, false,
		func(_ context.Context, in insertBlockIn) *sdk.CallToolResult { return s.apply("insert_block", in, "") })
	add(s, "Edit", "update_block",
		"Change fields of a web block (design: fill, ink, pad, gap, typeRole, weight, align; content per type), or of one child with childId (link label, feature title/body). Color fields take token keys, pad/gap take space indices 0-7. Refuses toId fields (use set_link_target), links/items lists (use upsert_child), and fields the block type does not have.",
		nil, false, func(_ context.Context, in updateBlockIn) *sdk.CallToolResult { return s.apply("update_block", in, "") })
	add(s, "Edit", "upsert_child",
		"Append a child to a nav/footer (a link {label}) or features block (an item {title, body}), or update one with childId. Refuses toId (use set_link_target) and blocks without children.",
		nil, false, func(_ context.Context, in upsertChildIn) *sdk.CallToolResult { return s.apply("upsert_child", in, "") })
	add(s, "Edit", "delete_child",
		"Delete a link or feature item from a block; a link's edge is removed when no other link still uses it. Refuses a block's main button.",
		nil, false, func(_ context.Context, in childIn) *sdk.CallToolResult { return s.apply("delete_child", in, "") })
	add(s, "Edit", "reorder_blocks",
		"Reorder the blocks of a web page. Input blockIds must list every block of the page exactly once.",
		nil, false, func(_ context.Context, in reorderBlocksIn) *sdk.CallToolResult {
			return s.apply("reorder_blocks", in, "")
		})
	add(s, "Edit", "duplicate_block",
		"Deep-copy a block with new ids directly after it; copied link targets keep their edges. Fails on unknown ids.",
		nil, false, func(_ context.Context, in blockIn) *sdk.CallToolResult { return s.apply("duplicate_block", in, "") })
	add(s, "Edit", "delete_block",
		"Delete a block, its links' edges (when unused elsewhere on the page), and pins on it. Fails on unknown ids.",
		nil, false, func(_ context.Context, in blockIn) *sdk.CallToolResult { return s.apply("delete_block", in, "") })
	add(s, "Edit", "update_screen",
		"Change fields of a mobile screen: name, title, body, archetype, layout, fill, ink, typeRole, weight, align, pad, gap, and cta ({label, size sm|md|lg, fill, ink}, or null to remove it). CTA heights: sm 36, md 44, lg 52 px (44 passes the tap check). Refuses toId fields (use set_link_target) and hotspots (use upsert_hotspot).",
		nil, false, func(_ context.Context, in updateScreenIn) *sdk.CallToolResult {
			return s.apply("update_screen", in, "")
		})
	add(s, "Edit", "upsert_hotspot",
		"Create a tappable hotspot on a mobile screen (omit hotspotId) or move/resize/relabel one; x, y, w, h are screen px within 390x844 and are clamped. Targets are set with set_link_target. Fails if nodeId is a web page.",
		nil, false, func(_ context.Context, in upsertHotspotIn) *sdk.CallToolResult {
			return s.apply("upsert_hotspot", in, "")
		})
	add(s, "Edit", "delete_hotspot",
		"Delete a hotspot and its edge. Fails on unknown ids.",
		nil, false, func(_ context.Context, in hotspotIn) *sdk.CallToolResult { return s.apply("delete_hotspot", in, "") })
	add(s, "Edit", "set_link_target",
		"Point a link, CTA, or hotspot at a node (or clear it with toId null); this creates, updates, or deletes the matching edge {fromId: nodeId, toId, label}. A link to its own page is allowed and makes no edge. Fails when toId is in the other channel (edges connect web to web or mobile to mobile) or ids are unknown.",
		map[string][]string{"owner.field": {"cta", "secondary", "button"}}, false,
		func(_ context.Context, in setLinkTargetIn) *sdk.CallToolResult {
			return s.apply("set_link_target", in, "")
		})
	add(s, "Edit", "upsert_edge",
		"Create a storyboard edge (fromId, toId, label; omit id) or relabel/retarget one (id given); a relabel also renames the link that owns it. Kind is cross whenever the nodes are in different flows (read-only), otherwise primary, alt, or back. Fails across channels, on a self edge, on a duplicate (fromId, toId, label), or on a kind that contradicts the flows.",
		map[string][]string{"kind": enumEdgeKind}, false,
		func(_ context.Context, in upsertEdgeIn) *sdk.CallToolResult { return s.apply("upsert_edge", in, "") })
	add(s, "Edit", "delete_edge",
		"Delete an edge and clear the toId of the link, CTA, or hotspot that owned it. Fails on an unknown id.",
		nil, false, func(_ context.Context, in edgeIn) *sdk.CallToolResult { return s.apply("delete_edge", in, "") })
	add(s, "Edit", "add_shape",
		"Add a shape to the SVG drawing: rect/ellipse {x,y,w,h,stroke,fill,sw}, line/arrow {x1,y1,x2,y2,stroke,sw}, text {x,y,text,role,ink}, pen {points,stroke,sw}. Colors are token keys; sw is 1, 1.5, 2, or 3. Fails on missing sizes or bad values.",
		map[string][]string{"shape.type": enumShape}, false,
		func(_ context.Context, in addShapeIn) *sdk.CallToolResult { return s.apply("add_shape", in, "") })
	add(s, "Edit", "update_shape",
		"Change fields of a shape (only fields of its type). Fails on unknown ids, fields, or values.",
		nil, false, func(_ context.Context, in updateShapeIn) *sdk.CallToolResult { return s.apply("update_shape", in, "") })
	add(s, "Edit", "duplicate_shape",
		"Copy a shape offset by +16/+16. Fails on an unknown id.",
		nil, false, func(_ context.Context, in shapeIDIn) *sdk.CallToolResult { return s.apply("duplicate_shape", in, "") })
	add(s, "Edit", "delete_shape",
		"Delete a shape (draw pins stay). Fails on an unknown id.",
		nil, false, func(_ context.Context, in shapeIDIn) *sdk.CallToolResult { return s.apply("delete_shape", in, "") })
	add(s, "Edit", "set_doc",
		"Replace the DESIGN.md markdown (read it with get_doc first and send the full text). Never fails.",
		nil, false, func(_ context.Context, in setDocIn) *sdk.CallToolResult { return s.apply("set_doc", in, "") })
	add(s, "Edit", "add_pin",
		"Add a numbered review pin (issue, suggestion, question) on a page or screen (target node, optional blockId; x,y fractions 0..1), a storyboard thumbnail (target board), or the drawing (target draw; world px). Numbers are never reused. Fails on an empty body or unknown ids.",
		map[string][]string{"kind": enumPinKind, "target.type": {"node", "board", "draw"}}, false,
		func(_ context.Context, in addPinIn) *sdk.CallToolResult { return s.apply("add_pin", in, "") })
	add(s, "Edit", "update_pin",
		"Change a pin: kind, body, resolved (true resolves, false reopens), or position. Fails on an empty body or an unknown id.",
		map[string][]string{"kind": enumPinKind}, false,
		func(_ context.Context, in updatePinIn) *sdk.CallToolResult { return s.apply("update_pin", in, "") })
	add(s, "Edit", "delete_pin",
		"Delete a pin; its number is never reused. Fails on an unknown id.",
		nil, false, func(_ context.Context, in pinIn) *sdk.CallToolResult { return s.apply("delete_pin", in, "") })

	// Output
	add(s, "Output", "export",
		"Generate an export from project data and return its text: page (one web page as standalone HTML), web-flows (all web pages in one HTML file), mobile-flow (a playable phone HTML of one flow), draw-svg, or design-md (DESIGN.md with generated Flows and Open issues). write=true also saves it into the project (DESIGN.md at the root, others under .agentcanvas/exports/) and returns the path. Fails on an unknown kind or id.",
		map[string][]string{"kind": enumExportKind}, false, s.export)

	// Packages
	add(s, "Packages", "skills_install",
		"Install a SKILL.md package folder (SKILL.md with name/description frontmatter plus skill.json) into the user's skill store; validated first, all-or-nothing. Fails on an invalid package, the reserved name agentcanvas, or an existing name without force.",
		nil, false, func(_ context.Context, in skillsInstallIn) *sdk.CallToolResult {
			p := in.Path
			if p != "" && !filepath.IsAbs(p) {
				// SPEC: relative skill paths resolve against the project directory, not the server's cwd.
				p = filepath.Join(st.Dir(), p)
			}
			info, err := s.o.Skills.Install(p, in.Force)
			if err != nil {
				return refused(err.Error(), "fix the package (see the agentcanvas skill for the format) or pass force: true to replace")
			}
			sum := fmt.Sprintf("Installed skill %s %s", info.Name, info.Version)
			st.Notify("mcp", sum)
			return text(marshal(map[string]any{"ok": true, "summary": sum, "skill": info}))
		})
	add(s, "Packages", "skills_remove",
		"Remove an installed skill by name. Fails for the built-in agentcanvas skill or a name that is not installed.",
		nil, false, func(_ context.Context, in skillsRemoveIn) *sdk.CallToolResult {
			if err := s.o.Skills.Remove(in.Name); err != nil {
				return refused(err.Error(), "call skills_list for installed names")
			}
			sum := "Removed skill " + in.Name
			st.Notify("mcp", sum)
			return text(marshal(map[string]any{"ok": true, "summary": sum}))
		})
}

func (s *Server) export(_ context.Context, in exportIn) *sdk.CallToolResult {
	p, view, _ := s.o.Store.Snapshot()
	q := url.Values{}
	nodeID, flowID := in.NodeID, in.FlowID
	if in.Kind == "page" && nodeID == "" {
		nodeID = string(view.CurrentNodeID.Web)
	}
	if in.Kind == "mobile-flow" && flowID == "" {
		if n := p.Node(string(view.CurrentNodeID.Mobile)); n != nil {
			flowID = n.FlowID
		}
	}
	if nodeID != "" {
		q.Set("nodeId", nodeID)
	}
	if flowID != "" {
		q.Set("flowId", flowID)
	}
	f, err := export.Generate(p, in.Kind, q)
	if err != nil {
		return refused(err.Error(), "use kind page, web-flows, mobile-flow, draw-svg, or design-md with ids from get_canvas")
	}
	head := fmt.Sprintf("Exported %s (%d bytes)", f.Name, len(f.Body))
	if in.Write {
		path, err := export.Write(s.o.Store.Dir(), in.Kind, f)
		if err != nil {
			return refused("could not write "+f.Name+": "+err.Error(), "check the project directory is writable")
		}
		head = fmt.Sprintf("Wrote %s to %s", f.Name, path)
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: head}, &sdk.TextContent{Text: string(f.Body)}}}
}
