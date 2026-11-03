# Infinite Canvas Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the four-tab, three-column studio with one infinite pan/zoom canvas that shows web pages, mobile screens and diagram shapes as artboards, edited in place, under floating Stitch-style chrome.

**Architecture:** The server stays the source of truth; the Go model gains three additive things (node positions, a persisted canvas viewport, an "entered" node) and three actions (`move_node`, `tidy_flow`, `set_canvas_view`, `set_entered`). The UI in `web/index.html` gets a new `Canvas` editor built on a `useViewport` hook lifted from `DrawEditor`, and new floating chrome (left panel, tool rail, contextual inspector, zoom pill) replacing `TopBar` context strips, `LeftPanel` and the fixed `Inspector`. Existing renderers (`renderBlock`, `renderScreen`), panels (`ContentPanel`, `DesignPanel`, `ReviewPanel`, `EdgePanel`), pins (`PinsLayer`) and overlays (`SpecsOverlay`, `GridOverlay`) are reused unchanged or with small prop additions.

**Tech Stack:** Go 1.25 (`internal/studio`, `internal/mcp`), React 18 + Babel standalone + Tailwind Play in a single `web/index.html` (no bundler, no npm), native SVG.

**Spec:** `docs/superpowers/specs/2026-11-03-infinite-canvas-redesign.md` (mockup: `docs/superpowers/specs/assets/infinite-canvas-mockup.html`). Product spec being amended: `docs/final-requeriments.md`.

## Global Constraints

- `web/index.html` keeps exactly one `<script type="text/babel" data-presets="react">` block; CI compiles it with the vendored Babel. No new vendored assets, fonts, or network hosts. Inter 13px for UI, Fraunces only for the wordmark and designed content.
- Server `Tabs = {web, story, draw, doc}` and per-tab undo stay. The UI never shows tabs; it maps the Canvas | DESIGN.md switch to `set_tab` (`doc` vs the last canvas tab) and lets the server move `view.tab` as selections change channel. The client's `act()` keeps sending `tab: state.view.tab`.
- All model and action changes are additive. Existing MCP tools keep their names and payloads; `project.json` written by the old version must load (`pos`, `canvas` absent means auto layout and fit-all).
- Tokens (spec §4.3): `canvas #F3F2EF`, `dot #D9D6CF`, `panel #FFFFFF`, `hair #E3E0D9`, `ink #1C1B19`, `muted #7A766D`, `faint #A8A49B`, `bar #1C1B19`, `accent #D8432A`, `accentSoft #FBE8E3`, `edge #8E8A80`, `ok #2E7D4F`, `warn #B23A1E`. Radii: 12 panels, 8 controls, 6 chips, 18 phone artboards, 0 web artboards. Zoom range 0.1 to 4. Motion: 120ms hover, 180ms panels, 220ms zoom-to-artboard, honoring `prefers-reduced-motion`.
- Interaction budget: under 100ms for any action, 60fps pan/zoom on the Hearth sample, zero console errors.
- Every §17 capability in `docs/final-requeriments.md` keeps working (rows re-worded in Task 1).
- Commit after every task on branch `feat/infinite-canvas`; never commit to `main` directly.

## Review Focus

1. **Loading an old `project.json`** (no `pos`, no `canvas`): every node must auto-place and the viewport must fit all artboards; nothing may throw on `canvas.view` being undefined. Pinned in Task 4 (`TestCanvasViewDefaults`) and Task 7 (manual load of the pre-redesign Hearth file).
2. **Undo of `move_node` across channels**: moving a mobile screen, then ⌘Z with a web node selected, must not undo the move (histories are per channel). Pinned in Task 3 (`TestMoveNodeHistoryIsChannel`).
3. **Deleting the entered node** (by the agent or by Delete in Layers) must leave edit mode cleanly: `view.entered` cleared by `fixView`, canvas back at the previous viewport, no stale `data-block-id` lookups. Pinned in Task 4 (`TestEnteredClearedOnDelete`) and Task 14 step 7.
4. **Pin drag at non-100% zoom**: a pin dragged on an artboard at 40% must land under the cursor, not 2.5× too far. Pinned in Task 9 step 4.
5. **Pan/zoom with a text field focused** (inspector input, pin popover): wheel over the canvas must zoom, but typing `h`, `v`, `r` into an input must never switch tools; Space in a textarea must insert a space. Pinned in Task 12 step 6.

---

### Task 1: Amend the product spec

**Files:**
- Modify: `docs/final-requeriments.md` §8.1 (584–636), §8.2 (638–656), §8.3 "Canvas" paragraph (659), §8.4 "Canvas" and "Filmstrip" paragraphs (694–698), §8.5 first bullet (714), §8.8 (738–746), §11.1 (834–850), §12.3 table (983–1027), §19 (1228–1232), §17 rows 2, 6, 14, 24, 27, 31, 32.

**Interfaces:**
- Produces: the normative text later tasks and the verification walkthrough cite. No code.

- [ ] **Step 1: Rewrite §8.1 to describe the shell**

Replace the layout block and the "TopBar context controls by tab" table with the shell from spec §4.1: 44px bar (wordmark, project name, Canvas | DESIGN.md, agent pill, undo/redo, Preview, Export, Skills), floating left panel (Layers / Activity / Review, prompt composer, `⌘\` collapse), right tool rail (Select V, Hand H, Add page P, Add screen M, Rect R, Ellipse O, Line L, Arrow A, Pen, Text T, Pin C, Tidy ⇧T), contextual inspector (`⌘.`), zoom pill, hint strip. Replace the "Visual direction" paragraph with the token table from Global Constraints.

- [ ] **Step 2: Rewrite §8.2 as "Canvas and flow groups"**

Keep lane order (BFS), ports, edge creation, edge styles, labels, offsets, hit test, Edge inspector, pins on artboards. Replace "Auto layout, deterministic, no manual node positions" with: auto placement by default; `pos {x, y}` set by dragging (`move_node`); `tidy_flow` clears positions; groups are bounding boxes of their nodes plus 40px padding; web groups stack at x = 0, mobile groups stack in a column 400px right of the widest web group, the diagram group sits 200px below the last web group. Thumbnail sizes become artboard sizes: web at the breakpoint width and natural height, mobile 390×844 with an 18px radius 1px frame. Ports sit at `y + min(h, 400) / 2`. Levels of detail: under 25% zoom a flat card with the name.

- [ ] **Step 3: Update §8.3, §8.4, §8.5, §8.8**

§8.3 Canvas: "the page is an artboard on the canvas; double-click, Enter or ⌘1 enters it (viewport animates to fit, other artboards dim to 60%); the contextual toolbar above it holds breakpoint, Grid, Specs, Preview, Duplicate, + Add block, ⋯". §8.4: the 24px bezel appears only in Play; remove the filmstrip paragraph (reorder via Layers drag or `reorder_nodes`). §8.5: shapes render in the diagram group of the same canvas; tools live on the rail. §8.8: Activity tab replaces the 3-second pill as the record (the pill stays, 3 s, in the bar); the prompt composer copies to the clipboard and calls no model.

- [ ] **Step 4: Update §11.1, §12.3, §19, §17**

§11.1 add rows: `H` hand, `⌘0` fit all, `⌘1` zoom to selection / enter, `Esc` leaves the entered artboard before clearing selection, `⌘\` left panel, `⌘.` inspector, `⇧T` tidy, `C` pin tool, draw keys work on the canvas. §12.3 add rows: `move_node {nodeId, x, y}` history channel, MCP `move_node`; `tidy_flow {flowId}` history channel, MCP `tidy_flow`; `set_canvas_view {x, y, zoom}` none, MCP `set_canvas_view`; `set_entered {nodeId | null}` none, MCP `set_entered`. §19 remove "manual storyboard node positioning". §17: row 2 drop the filmstrip clause; row 6 "When the canvas fits all, then the Marketing and Checkout groups show 2 artboards each and 12 curves, 9 vermillion"; row 14 "Preview hides the canvas chrome"; row 24 "on the Cart artboard" instead of "storyboard thumbnail"; row 27 "zooms to that pin"; rows 31, 32 "canvas" instead of "storyboard".

- [ ] **Step 5: Commit**

```bash
git checkout -b feat/infinite-canvas
git add docs/final-requeriments.md docs/superpowers
git commit -m "docs: amend the spec for the infinite canvas studio"
```

---

### Task 2: Design tokens and chrome CSS

**Files:**
- Modify: `web/index.html` lines 14–61 (`tailwind.config`, `<style>`), line 84–86 (`V`, `CH` constants).

**Interfaces:**
- Produces: Tailwind color names `canvas dot panel hair ink muted faint bar accent accentSoft edge ok warn` (`bg-panel`, `text-muted`, `border-hair` …); CSS variables `--ac-<token>`; classes `.ac-float` (panel: `bg panel, 1px hair, 12px radius, shadow 0 1px 2px rgba(20,18,14,.06), 0 8px 24px rgba(20,18,14,.08)`), `.ac-ibtn` (32px icon button, 8px radius, hover `#F3F2EF`, `.on` ink/white), `.ac-tip` (tooltip: ink bg, 11px, 6px radius, appears on `:hover` of the parent after 300ms), `.ac-art` (artboard: panel bg, 1px `#E6E3DC`, soft shadow), `.ac-art.sel` (2px accent outline), `.ac-dim` (opacity .6, 180ms), `.ac-zoomanim` (`transition: transform 220ms cubic-bezier(.2,.7,.2,1)`, disabled under `prefers-reduced-motion`). JS: `CH` re-pointed at the tokens, `V` unchanged.

- [ ] **Step 1: Add the tokens to `tailwind.config.theme.extend.colors` and as `:root` variables**

Keep the existing `paper ink muted accent wash line` names working (they are the project tokens' names too) by adding the new keys beside them; `ink`, `muted`, `accent` take the new values.

- [ ] **Step 2: Add the chrome classes listed in Interfaces to the `<style>` block; set `body { background: var(--ac-canvas) }`**

- [ ] **Step 3: Verify in the browser**

Run: `go run ./cmd/agent-canvas serve --dev --sample --port 47400` from the repo root; open it; confirm the page still renders and the console shows no errors (nothing visible changes yet except the body background).

- [ ] **Step 4: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): add design tokens and floating chrome styles"
```

---

### Task 3: Node positions, `move_node`, `tidy_flow`

**Files:**
- Modify: `internal/studio/types.go` (Node struct 82–100, `pageJSON` 112–118, `screenJSON` 120–138, `MarshalJSON` 140–153)
- Modify: `internal/studio/reduce.go` (handlers map 99–137; add handlers after `reorderNodes` 1047)
- Modify: `internal/studio/validate.go` (node checks in `Validate`)
- Test: `internal/studio/studio_test.go`

**Interfaces:**
- Produces: `type Point struct { X, Y float64 }` (json `x`, `y`); `Node.Pos *Point` (json `pos,omitempty`, emitted for both kinds); actions `move_node {nodeId string, x, y float64}` (rounds to integers, History = channel, Summary `Moved <name> to (x, y)`) and `tidy_flow {flowId string}` (sets `Pos = nil` on every node of the flow, History = channel, Summary `Tidied <flow>`; refuses with `invalid-payload` when no node had a position, message "nothing to tidy in <flow>").

- [ ] **Step 1: Write the failing tests**

```go
func TestMoveNodeAndTidy(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "move_node", map[string]any{"nodeId": "pg_shop", "x": 1500.4, "y": -20})
	n := s.project.Node("pg_shop")
	if n.Pos == nil || n.Pos.X != 1500 || n.Pos.Y != -20 { t.Fatalf("pos = %+v", n.Pos) }
	b, _ := json.Marshal(n)
	if !strings.Contains(string(b), `"pos":{"x":1500,"y":-20}`) { t.Fatalf("json %s", b) }
	act(t, s, "tidy_flow", map[string]any{"flowId": "fl_marketing"})
	if s.project.Node("pg_shop").Pos != nil { t.Fatal("tidy did not clear pos") }
	b, _ = json.Marshal(s.project.Node("pg_shop"))
	if strings.Contains(string(b), `"pos"`) { t.Fatalf("pos still emitted: %s", b) }
	refused(t, s, "invalid-payload", "tidy_flow", map[string]any{"flowId": "fl_marketing"})
	refused(t, s, "unknown-id", "move_node", map[string]any{"nodeId": "nope", "x": 0, "y": 0})
}

func TestMoveNodeHistoryIsChannel(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "move_node", map[string]any{"nodeId": "sc_welcome", "x": 10, "y": 10})
	if !s.historyFlags()["story"].CanUndo || s.historyFlags()["web"].CanUndo { t.Fatal("move must go to the story history only") }
	act(t, s, "undo", map[string]any{"tab": "story"})
	if s.project.Node("sc_welcome").Pos != nil { t.Fatal("undo did not restore pos") }
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/studio -run 'TestMoveNode' -v`
Expected: FAIL with `unknown action type "move_node"`.

- [ ] **Step 3: Implement `Point`, `Node.Pos`, the two JSON structs, `moveNode()` and `tidyFlow()`; register both in `handlers`; add `move_node`, `tidy_flow` to the §12.3-ordered list nothing else needs**

`Validate` accepts any finite position. `restoreSlice` needs no change (nodes are snapshotted whole).

- [ ] **Step 4: Run the whole package**

Run: `go test ./internal/studio`
Expected: PASS (including `TestSeedExact`, which must not change since the seed has no positions).

- [ ] **Step 5: Commit**

```bash
git add internal/studio
git commit -m "feat(studio): node positions with move_node and tidy_flow"
```

---

### Task 4: Canvas viewport, entered node, project name in state

**Files:**
- Modify: `internal/studio/types.go` (`Project` 49–59, `SharedView` 444–454, `normalize` 481–511)
- Modify: `internal/studio/reduce.go` (handlers; add `setCanvasView`, `setEntered` after `setDrawView` 660; `fixView` 2694)
- Modify: `internal/studio/history.go` (`index` 236–253: add `misc.add("canvas.view", p.Canvas.View)`)
- Modify: `internal/studio/store.go` (`State` 28–38: add `Name string`; `persistedView` 233–241 and `loadUI`/`writeFiles`: add `Entered`; `stateLocked` 360: set `Name: filepath.Base(s.dir)`)
- Test: `internal/studio/studio_test.go`

**Interfaces:**
- Produces: `Project.Canvas Canvas` with `Canvas{ View CanvasView }`, `CanvasView{ X, Y, Zoom float64 }` (json `canvas.view`; `Zoom == 0` means "unset, fit all"; `normalize` leaves it 0); action `set_canvas_view {x, y, zoom}` (clamps zoom to [0.1, 4] when non-zero, no history, Summary `Canvas view at N%`); `SharedView.Entered Ref` (json `entered`, persisted in `ui.json`); action `set_entered {nodeId string}` (empty or null leaves; History none; Summary `Entered <name>` / `Left <name>`; `set_entered` on a node also makes it current via `makeCurrent`); `fixView` clears `Entered` when the node is gone; `State.Name`.

- [ ] **Step 1: Write the failing tests**

```go
func TestCanvasViewDefaults(t *testing.T) {
	s, _ := newMem(t)
	if s.project.Canvas.View.Zoom != 0 { t.Fatal("fresh canvas view must be unset (zoom 0)") }
	act(t, s, "set_canvas_view", map[string]any{"x": 10, "y": 20, "zoom": 9})
	if v := s.project.Canvas.View; v.X != 10 || v.Y != 20 || v.Zoom != 4 { t.Fatalf("view %+v", v) }
	if s.historyFlags()["web"].CanUndo || s.historyFlags()["draw"].CanUndo { t.Fatal("view must not be undoable") }
	old := []byte(`{"key":"` + projectKey + `","project":` + string(mustJSON(SampleProject("x"))) + `}`) // no "canvas" key
	p, err := parseProject(old)
	if err != nil || p.Canvas.View.Zoom != 0 { t.Fatalf("old project: %v %+v", err, p.Canvas) }
}

func TestEnteredClearedOnDelete(t *testing.T) {
	s, _ := newMem(t)
	act(t, s, "set_entered", map[string]any{"nodeId": "pg_shop"})
	if s.view.Entered != "pg_shop" || s.view.CurrentNodeID.Web != "pg_shop" { t.Fatalf("view %+v", s.view) }
	act(t, s, "delete_node", map[string]any{"nodeId": "pg_shop"})
	if s.view.Entered != "" { t.Fatal("entered must clear when the node is deleted") }
	act(t, s, "set_entered", map[string]any{"nodeId": ""})
	if s.view.Entered != "" { t.Fatal("empty nodeId must leave") }
	if s.StateJSON("ui", "") == nil || !strings.Contains(string(s.StateJSON("ui", "")), `"name":`) { t.Fatal("state must carry the project name") }
}
```

Use the existing sample constructor name from `seed.go` in place of `SampleProject` if it differs; `mustJSON` is a two-line test helper.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/studio -run 'TestCanvasView|TestEntered' -v`
Expected: FAIL with unknown action / missing field compile errors.

- [ ] **Step 3: Implement the types, handlers, `fixView` clause, persistence fields and `State.Name`**

- [ ] **Step 4: Run the package and the server tests**

Run: `go test ./internal/...`
Expected: PASS. `TestPersistenceRoundTrip` must still pass with `entered` in `ui.json`.

- [ ] **Step 5: Commit**

```bash
git add internal/studio
git commit -m "feat(studio): persisted canvas view, entered node, project name in state"
```

---

### Task 5: MCP tools for the new actions

**Files:**
- Modify: `internal/mcp/mcp.go` (input types near 306; tool registrations near 567–575 "Edit" group and 522 "Navigate" group)
- Modify: `skills/agentcanvas/SKILL.md` (tool list, if it enumerates tools)
- Test: `internal/mcp/mcp_test.go`

**Interfaces:**
- Produces: tools `move_node` (`moveNodeIn{NodeID string; X, Y float64}`), `tidy_flow` (`flowIn`), `set_canvas_view` (`canvasViewIn{X, Y, Zoom float64}`), `set_entered` (`setEnteredIn{NodeID string}`), each calling `s.apply("<action>", in, "")`. Descriptions: "Place a page or screen on the canvas at world coordinates (x, y). Omitted positions are auto-laid out; tidy_flow clears them." / "Clear manual positions in a flow so its nodes return to automatic lane layout." / "Pan and zoom the shared canvas (zoom 0.1 to 4)." / "Enter a page or screen for in-place editing in the browser, or pass an empty nodeId to leave."

- [ ] **Step 1: Write the failing test**

In the existing tools-list test, assert the four names are present; add a call test for `move_node` on `pg_shop` that checks the returned text contains `Moved`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/mcp -v -run 'Tools|MoveNode'`
Expected: FAIL (names missing).

- [ ] **Step 3: Register the four tools; update the guide text if it lists tools**

- [ ] **Step 4: Run all Go tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp skills
git commit -m "feat(mcp): expose move_node, tidy_flow, set_canvas_view, set_entered"
```

---

### Task 6: `useViewport` hook and the `Canvas` shell with auto layout

**Files:**
- Modify: `web/index.html`: Section 1 constants (add after `LANE` 89), Section 7 (new `useViewport`, `layoutCanvas`, `Canvas` before `WebEditor` 2303), Section 8 `App` (editor choice 2879; the `drawView` plumbing 2777–2791 becomes the canvas view plumbing).

**Interfaces:**
- Consumes: `state.project.canvas.view` (Task 4), `act('set_canvas_view', {x, y, zoom})`.
- Produces:
  - `const ART = { web: w => w, mobileW: 390, mobileH: 844, GROUP_PAD: 40, GAP_X: 120, GAP_Y: 160, COL_GAP: 400, PORT_DY: 200 }`, `ZOOM_MIN = 0.1`, `ZOOM_MAX = 4`, `LOD_FLAT = 0.25`.
  - `function layoutCanvas(p, bp, heights) → { pos: {[nodeId]: {x, y, w, h}}, groups: [{flow, x, y, w, h, nodes}], diagram: {x, y} }` where `heights[nodeId]` is the measured artboard height (default 800 for pages); nodes with `pos` use it, others fill left-to-right in `laneOrder` after the right-most placed node in that group; web groups stack from (0, 0), mobile groups in a column at `x = maxWebRight + COL_GAP`, `diagram` at `(0, lastWebBottom + GAP_Y)`.
  - `function useViewport(ref, initial) → { view: {x, y, zoom}, set(fn, commit), toWorld(cx, cy), fitTo(rect, pad, animate), zoomBy(f, cx, cy), panBy(dx, dy) }`: wheel zooms around the cursor (ctrl/meta or pinch) and pans otherwise (two-finger scroll), clamps zoom, calls `commit` through a 250ms debounce into `act('set_canvas_view')` exactly like `setDrawView` (App 2777–2791, moved here).
  - `function Canvas({ spaceHeld })` renders `<div ref data-canvas class="absolute inset-0 overflow-hidden">` with the dot grid as a CSS `radial-gradient` background whose `background-size` and `background-position` follow the view, and a world `<div class="ac-world" style="transform: translate(x,y) scale(z)">` containing `<Artboards/>` (Task 7), `<Connectors/>` (Task 8), `<Groups/>` and `<DrawLayer/>` (Task 9). On first mount with `canvas.view.zoom === 0` it calls `fitTo(bounds of all groups, 80)`.
  - `useS()` context gains `viewport` (the hook result) and loses `drawView/setDrawView`.

- [ ] **Step 1: Write a pure-function test in the browser console**

There is no JS test runner. Add `window.__ac = { layoutCanvas, laneOrder }` at the end of Section 1 (kept, it is useful for the walkthrough script) and verify in the console on the sample: `__ac.layoutCanvas(state.project, 'desktop', {}).groups.map(g => [g.flow.name, g.x, g.y])` gives Marketing at (0,0), Checkout below it, Onboarding and Account in a column to the right, and `pos.pg_shop.x === 1280 + 120`.

- [ ] **Step 2: Implement `layoutCanvas` and `useViewport`; implement `Canvas` with the world transform and placeholder grey rectangles for nodes (name only)**

- [ ] **Step 3: Wire `App`: when `view.tab !== 'doc'` render `<Canvas/>`, else `<DesignDoc/>`; remove `WebEditor`, `StoryEditor`, `StoryboardView`, `DrawEditor`, `StartChannel` usage (keep the functions for now; Task 14 deletes)**

- [ ] **Step 4: Verify**

Run the dev server; expected: the sample shows nine grey rectangles in four groups plus a diagram area; drag pans, wheel zooms around the cursor, reload keeps the view (check `project.json` has `canvas.view`), a blank project (`serve` in an empty temp dir) fits to its single empty page without errors.

- [ ] **Step 5: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): infinite canvas shell with viewport hook and auto layout"
```

---

### Task 7: Artboards with levels of detail, selection, drag to move

**Files:**
- Modify: `web/index.html` Section 7: add `Artboard`, `Artboards`, `useArtboardHeights` next to `Canvas`.

**Interfaces:**
- Consumes: `layoutCanvas` (Task 6), `renderBlock`/`renderScreen` (Section 4), `act('move_node')`, `act('set_selection')`, `act('set_entered')`.
- Produces:
  - `const Artboard = memo(({ node, box, zoom, bp, tokens, selected, entered, dimmed, onDown, onDbl }))` renders `<div class="ac-art" data-art-id={node.id} style={{left, top, width, height}}>` with: the name label `<div class="ac-art-name" style={{transform: scale(1/zoom)}}>` (★ for start, breakpoint px for pages); body = flat card when `zoom < LOD_FLAT`, else `renderBlock` for each block at `bp` in mode `'thumb'` (or `'edit'` when entered, Task 14) or `renderScreen` in a 390×844 box with 18px radius and `overflow: hidden`; `pointer-events: none` on the body unless entered; in/out port `<span class="ac-port" data-port="in|out">` at `y + min(h, 400)/2` counter-scaled.
  - `useArtboardHeights(worldRef) → heights` using one `ResizeObserver` on `[data-art-body]` elements, feeding `layoutCanvas`.
  - Drag: pointer down on a non-entered artboard with the Select tool starts a move after 4px; local override of `box` during drag; on release `act('move_node', {nodeId, x: Math.round(wx), y: Math.round(wy)})` where `wx, wy` come from `viewport.toWorld`; Alt disables 8px snap. Click selects (`set_selection {kind:'node'}`); double-click enters (`set_entered {nodeId}`), Task 14 makes entering do more.

- [ ] **Step 1: Implement `Artboard`, `Artboards`, `useArtboardHeights` and replace the grey placeholders**

- [ ] **Step 2: Verify rendering**

Dev server, sample: at fit zoom the Hearth pages render with their real blocks, phones render with the 18px frame and no bezel; zooming under 25% switches to flat cards; names stay 12px at every zoom.

- [ ] **Step 3: Verify moving**

Drag `Shop` 300px right; reload; it stays. `tidy_flow` via `curl -s localhost:47400/api/act -d '{"type":"tidy_flow","payload":{"flowId":"fl_marketing"},"tab":"web"}' -H 'Content-Type: application/json'` snaps it back. Select a mobile screen, press ⌘Z: the toast says nothing to undo in Story (the move was web).

- [ ] **Step 4: Load the pre-redesign sample**

Copy `.agentcanvas/project.json` from a `git stash`-free checkout of `main` (or run the old binary in a temp dir) into a temp project; serve; expected: all nodes auto-place, console clean.

- [ ] **Step 5: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): artboards with level of detail and drag to place"
```

---

### Task 8: Connectors in world space

**Files:**
- Modify: `web/index.html` Section 7: `Connectors`, `edgeGeom`, port drag; reuse `EdgeLabelInput` (2219) and `cubicAt` (2042).

**Interfaces:**
- Consumes: `pos` boxes from `layoutCanvas`, `ART.PORT_DY`, `act('upsert_edge')`, `act('set_selection', {kind:'edge'})`.
- Produces:
  - `function edgeGeom(a, b, kind, off) → { d, mx, my }` where `a`, `b` are boxes; start `(a.x + a.w, a.y + min(a.h,400)/2 + off)`, end `(b.x, b.y + min(b.h,400)/2 + off)`; forward edges `C` with `dx = max(80, (x2 - x1)/2)`; back and leftward edges bow below the lower box by `GAP_Y * 0.4 + off`; same from/to pairs offset by `off = (i - (n-1)/2) * 28` world units.
  - `function Connectors({ layout, zoom })` renders one `<svg class="ac-edges" style="position:absolute; overflow:visible; pointer-events:none">` inside the world with `vectorEffect: non-scaling-stroke` paths: colors `edge` token (primary), dashed `6 4` (alt), dotted `2 4` (back), `accent` 2px (cross); arrow markers `ac-arrow-edge` and `ac-arrow-v`; a transparent 12/zoom hit stroke selects; label pill `<g>` counter-scaled (`transform: translate(mx,my) scale(1/zoom)`) with white fill and `#E6E3DC` stroke, accent fill when selected; hovering raises the edge (`stroke-width` +0.5 and `opacity` 1 vs 0.9 for others).
  - Port drag: pointer down on `[data-port="out"]` tracks a dashed accent preview path to the cursor in world units; on release `document.elementsFromPoint` finds `[data-art-id]`; if a different node, shows `EdgeLabelInput` at the drop point (screen px, outside the world transform) then `act('upsert_edge', {fromId, toId, label})`.

- [ ] **Step 1: Implement `edgeGeom`, `Connectors`, the port drag**

- [ ] **Step 2: Verify on the sample (spec §17 row 6, 7, 8, 31, 32)**

Twelve web edges visible, nine vermillion; `sc_welcome` shows "Get started" and "Skip" with offset labels; drag `pg_shop` out-port onto `pg_cart`, type "Buy now", Enter: a vermillion cross edge appears and the Edge inspector (Task 13 makes it float; for now the old fixed inspector) shows kind `cross` read-only. Move `Confirm` far below `Cart`: the edge still leaves right and enters left without crossing either artboard.

- [ ] **Step 3: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): world-space connectors with port drag"
```

---

### Task 9: Flow groups, diagram layer, pins in world space

**Files:**
- Modify: `web/index.html` Section 7: `Groups`, `DrawLayer` (ported from `DrawEditor` 2514–2698), `CanvasPins`; Section 6 `PinMarker` (1802) gains a `scale` prop.

**Interfaces:**
- Consumes: `layout.groups`, `layout.diagram`, `ui.tool` (replaces `ui.drawTool`; values `select | hand | rect | ellipse | line | arrow | text | pen | pin`), `act('add_shape' | 'update_shape' | 'add_pin' | 'update_pin')`.
- Produces:
  - `Groups({ layout, zoom })`: one `<div class="ac-group">` per group (dashed `#D2CFC7` 1px, 16px radius, `rgba(255,255,255,.28)`), label `<button class="ac-group-label">Name · N pages · web</button>` counter-scaled; click selects nothing but opens the flow in the inspector via a local `ui.flowId`; double-click renames inline (`InlineRename`, `upsert_flow`); hover shows `+ page/screen` and `Tidy` buttons (`upsert_node {flowId}`, `tidy_flow`).
  - `DrawLayer({ viewport, origin })`: the `DrawEditor` SVG moved inside the world `<svg>` under `<g transform="translate(origin.x, origin.y)">`; `toWorld` subtracts `origin`; all tools, handles, snap, `TextPlacer` and keyboard behavior unchanged; the dot grid pattern is removed (the canvas has it); shapes stay in diagram coordinates, so agents and the SVG export are unaffected.
  - `CanvasPins`: board pins (`target.type === 'board'`) at `pos[nodeId] + fraction * box`, draw pins at `diagram origin + (x, y)`, node pins inside the artboard body as today (`WebPage` 2379–2391, `StoryPage` 2441–2444 logic moved into `Artboard`); `PinMarker` receives `scale = 1/zoom` and applies it to its drag deltas and to a `transform: scale()` so it stays 22px on screen; `coords()` goes through `viewport.toWorld`.
  - Annotate: the `pin` tool replaces `ui.annotate`; clicking an artboard body places a node/board pin exactly as `onAnnotate` does today (fractions of block or frame; board when not entered), clicking the diagram places a draw pin; `DraftPin` renders in screen space.

- [ ] **Step 1: Implement `Groups`**

- [ ] **Step 2: Port `DrawEditor` into `DrawLayer`; delete `DrawEditor`, `DrawPanel`**

- [ ] **Step 3: Implement `CanvasPins` and the `scale` prop**

- [ ] **Step 4: Verify pins (spec §17 rows 24, 26)**

At 40% zoom: place a pin on the Cart artboard, on `sc_craft`, on the Home hero block, and on the diagram; each shows the next number and sits under the cursor; drag the diagram pin 100px right at 40%: it lands under the cursor (world delta 250); reload keeps all four; Resolve greys it, Delete removes it.

- [ ] **Step 5: Verify drawing (spec §17 rows 3, 20)**

Press R, drag in the diagram group: a rect with corners at multiples of 8; with Alt, not; the five seed shapes render in the diagram group; `curl /api/export/draw-svg` is byte-identical before and after this task.

- [ ] **Step 6: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): flow groups, diagram layer and pins on the canvas"
```

---

### Task 10: Top bar

**Files:**
- Modify: `web/index.html` Section 5: replace `TabSwitch` (739), `TopBar` (791–854), `AnnotateControls` (777); keep `IssueBadge`, `ExportMenu`, `DarkSeg`.

**Interfaces:**
- Consumes: `state.name` (Task 4), `state.history[tab]`, `act('set_tab')`, `act('set_preview')`, `ui.leftTab`, `setUi`.
- Produces: `TopBar` = 44px `bg-bar`: wordmark (Fraunces 15/600), `/ <name> ▾` (dropdown: Load the Hearth sample…, Start over (blank)…; both moved out of `ExportMenu`), `DarkSeg` Canvas | DESIGN.md (`act('set_tab', {tab: 'doc'})` or back to `uiRef.lastCanvasTab || 'web'`), spacer, `AgentPill` (existing 3s pill, now `● <summary>` with a green dot), `IssueBadge` (opens the left panel Review tab), undo/redo `ac-ibtn` with tooltips, `▶ Preview` (web when the current selection or entered node is a page, Play otherwise), `ExportMenu`, `Skills · N` (opens `SkillsSheet`, Task 15). No per-tab controls.

- [ ] **Step 1: Implement; delete `TabSwitch`, `AnnotateControls`**

- [ ] **Step 2: Verify**

The bar is 44px, shows the project directory name, the switch toggles the DESIGN.md editor and back without losing the canvas viewport; undo/redo disable per the server flags; Export still downloads; "Start over (blank)…" asks and resets.

- [ ] **Step 3: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): minimal top bar with canvas/doc switch"
```

---

### Task 11: Left panel: Layers, Activity, Review, prompt composer

**Files:**
- Modify: `web/index.html` Section 2 `useStudio` (`accept` 254–268: record activity), Section 5: replace `LeftPanel`, `Palette`, `Outline`, `DocRail` (940–1179) with `LeftPanel`, `Layers`, `Activity`, `PromptComposer`; `ReviewPanel` (1721) moves here unchanged.

**Interfaces:**
- Consumes: `laneOrder`, `useFailing`, `InlineRename`, `ReviewPanel`, `act('set_selection' | 'set_entered' | 'upsert_flow' | 'upsert_node' | 'delete_node' | 'reorder_nodes')`, `state.name`.
- Produces:
  - `useStudio()` returns `activity: [{rev, source, summary, at}]`, newest first, capped at 200, appended in `accept()` whenever `summary` is non-empty (both `ui` and `mcp` sources; the agent pill still only reacts to `mcp`).
  - `ui.leftTab: 'layers' | 'activity' | 'review'`, `ui.leftOpen: boolean` (default true, `⌘\`).
  - `LeftPanel`: `.ac-float` at `left 14 / top 58 / bottom 14 / width 272`, tab strip with counts (Activity: entries since the tab was last viewed; Review: `state.openIssues`), body, `PromptComposer` footer. Collapsed: a 40px rail with three icon buttons.
  - `Layers`: the `Outline` tree for both channels under "Web · N pages" and "Mobile · N screens" headers, plus "Diagram · N shapes" listing shapes (from `DrawPanel`), same affordances as `Outline`; a node row click selects and calls `viewport.fitTo(box, 120, true)`; double-click enters. HTML5 drag between node rows of the same flow calls `reorder_nodes` (the filmstrip's `reorder` from `StoryPage` 2459–2464).
  - `Activity`: rows `<avatar> <summary> <time ago>`; avatar `CC`-style initials for `mcp` ("Agent"), `You` for `ui`; click selects the first id mentioned in the summary when `state.project` has it (best effort: match `"…"` quoted name against node names), else no-op.
  - `PromptComposer`: textarea + `Copy prompt` (⌘K focuses it); builds `Project "<name>" in AgentCanvas (MCP: <origin>/mcp). Selected: <selection label or none>. Open issues: <n>. Request: <text>` and `navigator.clipboard.writeText`; toast "Copied. Paste it into Claude Code or Codex." Nothing is sent anywhere.

- [ ] **Step 1: Record activity in `accept()`; implement `LeftPanel`, `Layers`, `Activity`, `PromptComposer`; delete `Palette`, `Outline`, `DocRail`, `DrawPanel`**

- [ ] **Step 2: Verify (spec §17 rows 5, 10, 16, 27)**

Layers lists Marketing, Checkout, Onboarding, Account with their nodes; clicking "Home › Nav › Cart" outlines that link on the canvas (after Task 14; for now selects it); `+ Flow`, rename to "Support", add a page, delete the flow leaves no orphans in `project.json`; `curl` an MCP-style action with `source` mcp (use `agent-canvas attach` or the HTTP MCP) and the Activity tab shows it within one frame; Review filter Question + Open shows only open questions and clicking a row zooms to the pin.

- [ ] **Step 3: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): floating project panel with layers, activity, review and prompt composer"
```

---

### Task 12: Tool rail, zoom pill, hint strip, empty-project card, keyboard map

**Files:**
- Modify: `web/index.html` Section 5: add `ToolRail`, `ZoomPill`, `HintStrip`, `EmptyCanvas`; Section 8 `App` keydown handler (2798–2872).

**Interfaces:**
- Consumes: `ui.tool`, `viewport`, `layout` bounds, `act('upsert_node')`, `act('reset_project', {template: 'sample'})`.
- Produces:
  - `ToolRail`: `.ac-float` right 14, vertically centered, 44px wide, buttons (inline 16px SVG icons, 1.6 stroke): Select V, Hand H · Add web page P (`upsert_node` in the selected/first web flow), Add mobile screen M · Rect R, Ellipse O, Line L, Arrow A, Pen, Text T · Pin C, Tidy ⇧T (tidies the selected node's flow, or every flow with a confirm). Each has `.ac-tip` "Label <kbd>".
  - `ZoomPill`: `.ac-float` bottom 14 right 14: Fit ⌘0, −, `NN%` (click resets to 100%), +, ? (popover with the keyboard map table from §11.1).
  - `HintStrip`: bottom center pill with one context line: default "Drag to pan · scroll to zoom · double-click a page to edit it"; when entered "Esc leaves <name>"; hidden after 5 sessions via `localStorage['ac-hints']`.
  - `EmptyCanvas`: when `project.nodes.length === 0 && draw.shapes.length === 0`, a centered `.ac-float` card: "Add a web page", "Add a mobile screen", "Load the Hearth sample", "Set up an agent" (copies the README install prompt).
  - Keyboard (window `keydown`, one listener, unchanged structure): keep every existing branch; tool keys `v h r o l a t p c` work whenever not typing and not in preview; `Space` pans (sets `spaceHeld`); `⌘0` fit all, `⌘1` fit selection (enter if a node, Task 14), `⌘\` toggle left panel, `⌘.` toggle inspector, `⇧T` tidy; `Esc` order: preview → typing blur → skills sheet → entered (leave) → tool back to select → clear selection.

- [ ] **Step 1: Implement the four components and the keyboard additions**

- [ ] **Step 2: Verify tools and zoom**

Each rail button and key switches `ui.tool`; the pill percentage matches the viewport; ⌘0 fits all nine artboards with 80px padding; the ? popover lists the shortcuts.

- [ ] **Step 3: Verify the empty project**

`serve` in an empty temp dir: the card appears over an empty canvas; "Add a web page" creates and selects a page and the card disappears; "Load the Hearth sample" asks and loads.

- [ ] **Step 4: Verify tooltips and hints**

Hovering a rail button for 300ms shows "Select V"; the hint strip reads the default line and switches when entered (Task 14).

- [ ] **Step 5: Verify keys versus typing (Review Focus 5)**

Focus the Name input in the inspector and type "hvr": the tool stays Select and the field reads "hvr"; press Space in the DESIGN.md textarea: a space is inserted, the canvas does not pan.

- [ ] **Step 6: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): tool rail, zoom pill, hints, empty state and keyboard map"
```

---

### Task 13: Contextual floating inspector

**Files:**
- Modify: `web/index.html` Section 5: `Inspector` (1182–1202), `ContentPanel` (1204), `DesignPanel` (1528), `NodeHeader` (1251); Section 6 `ReviewPanel` stays in the left panel.

**Interfaces:**
- Consumes: `view.selection`, `view.entered`, `ui.flowId` (Task 9), `ui.inspectorOpen` (`⌘.`).
- Produces: `Inspector` = `.ac-float` at `right 72 / top 58 / width 292 / max-height calc(100% - 140px)`, rendered only when there is a selection, an entered node, or `ui.flowId`; tabs Content / Design (Review moved out); `ContentPanel` and `DesignPanel` dispatch on the selection kind and the node kind instead of `view.tab`: block → `BlockContent`/`BlockDesign`; node or hotspot on a screen → `ScreenContent`/`ScreenDesign`; node that is a page → `NodeContent`; shape → `ShapeContent`/`ShapeDesign`; edge → `EdgePanel`; pin → nothing (popover); flow (`ui.flowId`) → a new `FlowContent` (name, start node select, channel read-only, Delete flow); `ProjectTokens` appears under Design for nodes and blocks as today and in a "Project" popover from the top bar when nothing is selected. `NodeContent` gains the breakpoint `Seg` (1280/768/390) and "Start page of flow" toggle. Slide-in 180ms (`.ac-float` + translateX), no layout shift of the canvas.

- [ ] **Step 1: Implement; move the breakpoint control into `NodeContent`**

- [ ] **Step 2: Verify (spec §17 rows 11, 12, 15, 17, 21, 22)**

Select `bk_home_hero` (after Task 14 by clicking; for now via Layers): Design shows typeRole display, weight, align, pad 8, gap 4, swatches; change accent to `#0055FF` in Project tokens: every CTA updates, cross edges and pins stay vermillion; `bk_shop_cta` shows "Fails AA" and ⚠ in Layers; `sc_profile` Tap targets lists "Sign out: 36px (min 44)"; `⌘.` hides and shows the panel; Esc clears the selection and the panel slides out.

- [ ] **Step 3: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): contextual floating inspector"
```

---

### Task 14: Edit in place

**Files:**
- Modify: `web/index.html` Section 7: `Artboard` (Task 7) edit mode, `ContextToolbar`, `enterNode`/`leaveNode` in `Canvas`; delete `WebEditor`, `WebPage`, `StoryEditor`, `StoryPage`, `StoryboardView`, `StartChannel`, `Thumb`, `Bezel` stays (Play only).

**Interfaces:**
- Consumes: `view.entered`, `act('set_entered')`, `viewport.fitTo`, `WebPage` internals (`onSelect` 2322, `onChipDown` 2329–2356, `onAnnotate` 2365, pin items 2379–2391), `StoryPage` internals (`onSelect`, `onHotspotEdit`, `onAnnotate`, specs 2448), `SpecsOverlay`, `GridOverlay`, `useRects`.
- Produces:
  - `Canvas.enterNode(id)`: stores the current view in `prevViewRef`, `act('set_entered', {nodeId: id})`, `viewport.fitTo(box, 100, true)`; `leaveNode()`: `act('set_entered', {nodeId: ''})`, restores `prevViewRef` animated. Triggered by double-click, Enter / ⌘1 on a selected node, Layers double-click, and by the server (`view.entered` changing from an agent): when `entered` changes and `prevViewRef` is empty, fit to it without storing.
  - `Artboard` in edit mode (`entered === node.id`): body `pointer-events: auto`, `mode: 'edit'`, block chips and the 2px drop line (`onChipDown`), nested child selection toggle (`onSelect`), hotspot move/resize (`onHotspotEdit`), `SpecsOverlay` when `view.showSpecs`, `GridOverlay` when `view.showGrid` (pages), pin placement on blocks/frame with the Pin tool, click on empty frame selects the node. Every other artboard gets `.ac-dim`. Clicking the canvas background while entered leaves.
  - `ContextToolbar({ box, zoom })`: `.ac-float` 34px positioned above the selected or entered artboard in screen space (`left = view.x + (box.x + box.w/2) * zoom`, `top = view.y + box.y * zoom - 44`, clamped into the viewport): pages: breakpoint `Seg` 1280/768/390 (`set_breakpoint`), Grid, Specs, ▶ Preview, `+ Add block ▾` (menu of `BLOCK_TYPES`, `insert_block` after the selected block), Duplicate (page: not available; block selected: `duplicate_block`), ⋯ (Rename, Set as start, Delete page); screens: Specs, ▶ Play, `+ Add screen` (archetype menu, `upsert_node {flowId, archetype, afterId}`), `+ Hotspot`, ⋯.
  - Delete key on a selected node works everywhere (the old "only on the board" guard goes); confirm when it has blocks or pins.

- [ ] **Step 1: Implement enter/leave and the edit-mode artboard; delete the old editors**

- [ ] **Step 2: Implement `ContextToolbar`**

- [ ] **Step 3: Verify entering (spec §17 rows 1, 13, 14, 16, 17, 18, 19)**

Double-click Home: the viewport animates to fit it, others dim, the hint reads "Esc leaves Home"; editing the hero headline in Content updates the canvas per keystroke; blur, ⌘Z restores, ⇧⌘Z re-applies; clicking the second feature card outlines only it (dashed) and Content edits its title; Specs at 1280 reads `52/56`, `12/16`, `8`; at 390 `32/38`, `12/16`, `16`; Grid overlays 12 columns and blocks still select; Preview hides every panel and the rail; Esc returns with Home entered; Esc again leaves.

- [ ] **Step 4: Verify screens (spec §17 rows 2, 9, 22)**

Enter `sc_welcome`: hotspot "Skip" shows as a dashed rectangle, drags and resizes, numeric fields follow; Play from Onboarding, tap "Allow and finish" on Notifications: Account › Home appears with two progress segments in the bezel.

- [ ] **Step 5: Verify block drag and keyboard**

Drag the Features chip above Hero: a 2px drop line shows; release reorders in one undo step; ⌘D duplicates the selected block; Delete removes a selected link child.

- [ ] **Step 6: Verify an agent entering**

`set_entered {nodeId: "pg_cart"}` through MCP: the browser zooms to Cart; `set_entered {nodeId: ""}`: it zooms back.

- [ ] **Step 7: Verify deleting the entered node (Review Focus 3)**

Enter Shop, delete it from Layers: edit mode ends, the viewport returns to the previous view, no console errors, Layers and the inspector show nothing stale.

- [ ] **Step 8: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): edit pages and screens in place with a contextual toolbar"
```

---

### Task 15: Preview restyle, skills sheet, token sweep, motion

**Files:**
- Modify: `web/index.html`: `PreviewStage` (2236–2298), `SkillsDrawer` → `SkillsSheet` (905–937), `Popover`, `Toast`, `Seg`, `IconButton`, `Pill`, `Section`, `inputCls`, `Field`, every remaining `#DDD5C4 #6B665C #232220 #F4EFE6 #FBF8F2 #EFE8DB #E7E0D2` literal in chrome (not in `renderBlock`/`renderScreen`, which use project tokens).

**Interfaces:**
- Produces: `PreviewStage` on `#141311` with the existing bottom pill restyled (`.ac-float`, 999 radius); `SkillsSheet` = `.ac-float` 360px from the right edge over the inspector area, same content, Esc closes; primitives use the Tailwind token names (`border-hair`, `text-muted`, `bg-panel`…), 8px radii, 30px inputs, focus ring `2px accent, offset 2px`; `.ac-zoomanim` applied to the world during `fitTo(…, animate)` only; `prefers-reduced-motion` disables it.

- [ ] **Step 1: Restyle `PreviewStage` and `SkillsSheet`**

- [ ] **Step 2: Replace chrome hex literals with token classes**

Run: `grep -c '#DDD5C4\|#6B665C\|#232220\|#F4EFE6\|#FBF8F2\|#EFE8DB\|#E7E0D2' web/index.html`
Expected: 0 outside Section 4 (renderers keep `CH.stage` for the Play bezel).

- [ ] **Step 3: Verify (spec §17 rows 14, 44)**

Preview hides all chrome and Esc exits; Skills sheet lists name, version, description, renders SKILL.md, has no install/remove/edit control; tab through the rail and inspector: every control shows the accent focus ring; with "Reduce motion" on in macOS, zoom-to-artboard jumps.

- [ ] **Step 4: Commit**

```bash
git add web/index.html
git commit -m "feat(ui): preview and skills restyle, token sweep, motion"
```

---

### Task 16: Verification walkthrough and report

**Files:**
- Create: `scripts/ui-shots.mjs` (puppeteer-core against `serve --dev --sample`; captures fit, entered page, entered screen, preview, blank project; requires a local `npm i puppeteer-core` outside the repo, documented in a header comment; not in CI)
- Modify: `docs/verification-report.md` (new §17/§18 pass), `README.md` "Quick start" paragraph (describe the canvas, Layers, Activity, the rail), `docs/index.html` feature copy if it names tabs.

- [ ] **Step 1: Run `make test` and the CI Babel compile locally**

Run: `make test && node -e '<the CI snippet from .github/workflows/ci.yml>'`
Expected: PASS, "web/index.html compiles".

- [ ] **Step 2: Walk every §17 row (1–33, 38, 44) and every §18 step (1–9, 11) in Chrome on the sample; record pass/fail with notes in `docs/verification-report.md`**

- [ ] **Step 3: Performance check**

With the Chrome performance panel recording, pan and zoom the sample for 10 seconds: no frame over 16ms attributable to React commits; selection and typing stay under 100ms; console has zero errors and zero React warnings.

- [ ] **Step 4: Update README and website copy; capture screenshots with `scripts/ui-shots.mjs` into the scratchpad for the PR description**

- [ ] **Step 5: Commit**

```bash
git add scripts docs README.md
git commit -m "docs: verify the infinite canvas studio against §17/§18"
```
