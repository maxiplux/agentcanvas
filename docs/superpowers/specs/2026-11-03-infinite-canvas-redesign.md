# AgentCanvas studio redesign: one clean, infinite canvas

Status: proposal, 2026-11-03. Nothing here is built yet.
Mockup: `docs/superpowers/specs/assets/infinite-canvas-mockup.html` (static, throwaway; open it in a browser, drag to pan, scroll to zoom).

## 1. Why

The studio today is a fixed three-column tool: a 48px top bar with four tabs (Web, Story, Draw, Doc), a 260px left panel, a scrolling middle area, and a 300px inspector. It works, but it feels like a form, not a design tool:

- **Mode switching everywhere.** Web vs Story, Page vs Storyboard, Draw as its own world. You can never see the whole product at once. Moving from "where is this page in the flow" to "edit this page" is a tab change, not a zoom.
- **The storyboard is a static picture, not a space.** No pan, no zoom, auto-layout only. With four pages the edges already cross each other in a vermillion tangle.
- **Chrome dominates.** Permanent panels take 560px of every viewport before any design is shown. Specs and grid overlays are on in the sample, so the first impression is red rulers over the page.
- **No sense of the agent.** The whole point of the product is that an agent and a person edit the same canvas, but the only trace of the agent is a pill that lives for three seconds.
- **The visual language fights the content.** Warm beige panels, hard 4px corners, heavy borders and a loud accent compete with the designs being made, which are themselves warm and serif. Tool and artifact look the same, so neither reads clearly.

The reference the user pointed at is Google Stitch: a quiet, light, infinite canvas where screens float as artboards; a minimal dark top bar; a conversational panel on the left; a floating tool rail on the right; zoom controls bottom right. The canvas is the product, the chrome gets out of the way.

**Intended outcome.** Open the studio and see the whole product at once: web pages, mobile screens and diagrams as artboards on one infinite surface, flows as light groups, links as calm connectors. Zoom into any artboard to edit it in place. Panels float and collapse. The agent's work shows up as a live feed. The studio looks like a design tool a designer would choose.

## 2. Goals, non-goals, anti-goals

**Goals**
1. One infinite canvas with pan and zoom for everything: web pages, mobile screens, diagram shapes, pins and connectors.
2. Edit in place: no Page vs Storyboard mode. Select an artboard, double-click or zoom to edit its blocks.
3. Chrome that floats and collapses: a left project panel, a right tool rail, a contextual inspector, a zoom pill. The canvas is never boxed in.
4. A calm visual system: neutral light surface, white artboards, one accent used sparingly, consistent 8/12px radii, hairline borders, soft elevation, tooltips with shortcuts.
5. Make the agent visible: an Activity feed of what agents changed, live, and a presence pill in the top bar.
6. Keep every capability in the §17 checklist of `docs/final-requeriments.md` working (pins on four surfaces, specs and grid overlays, breakpoints, play/preview, undo per tab, skills drawer, keyboard map, checks).
7. Stay within the frontend constraints of §4.3: one HTML file, the four vendored scripts plus fonts.css, no bundler, no canvas library, native SVG for drawing.

**Non-goals**
- A built-in chat with an LLM. The binary has no model. The left panel gets an agent *prompt composer* that copies a context-rich prompt to the clipboard for Claude Code or Codex; it does not call a model.
- Dark mode for the studio chrome (can follow later; the token system makes it cheap).
- Multi-user cursors or presence beyond the agent pill.
- New block types, new mobile archetypes, new export formats.

**Anti-goals (what would make this a failure even if it "works")**
- Breaking the MCP contract. Agents must keep driving the project through the same action catalog (§12.3), with additive changes only.
- A canvas that is slower than today. Pan and zoom must stay under 16ms per frame with the Hearth sample; the interaction budget stays at 100ms (§4.3).
- Prettier chrome with the same mode-switching. If the Page/Storyboard toggle survives, the redesign missed the point.
- Reintroducing clutter through floating panels: if every panel is open all the time, we only moved the problem.

## 3. Constraints

- `web/index.html` stays a single `text/babel` block compiled by Babel standalone; CI checks for exactly one such block and runs `make vendor-verify`. No new vendored assets without updating `Makefile` and the sha list. Inter and Fraunces are the only fonts.
- Server state remains the source of truth (`internal/studio`). The view model `SharedView` and `Tabs = {web, story, draw, doc}` are used by MCP; undo histories are per tab. The redesign keeps the tab concept in the model and hides it in the UI.
- The spec `docs/final-requeriments.md` §8.1 to §8.8 fixes today's layout and §8.2/§19 forbid manual storyboard positions. This proposal amends the spec; the amendment is part of the work (see §7, phase 0).
- `docs/verification-report.md` records a scripted Chrome pass over §17/§18. The new UI must pass an equivalent walkthrough; the script's selectors will change.

## 4. The design

### 4.1 Shell

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ 44px bar  AgentCanvas / Hearth ▾   [Canvas | DESIGN.md]     ● Claude Code · editing Shop  ↶ ↷  ▶ Preview  Export ▾ │
├──────────────────────────────────────────────────────────────────────────────┤
│ ╭ Project ───────╮                                              ╭ Inspector ─╮ ╭╮ │
│ │ Layers Activity│        infinite canvas (dot grid)            │ contextual │ │▸│ │
│ │ Review         │   ┌ Marketing ─────────┐  ┌ Onboarding ──┐   │ Content    │ │✋│ │
│ │                │   │ [Home]──▶[Shop]    │  │ [☐][☐][☐]    │   │ Design     │ │▭│ │
│ │ flows › nodes  │   └────────────────────┘  └──────────────┘   │ Review     │ │▯│ │
│ │                │   ┌ Checkout ──────────┐                     ╰────────────╯ │○│ │
│ │ ──────────     │   │ [Cart]──▶[Confirm] │                                    │↗│ │
│ │ Ask your agent │   └────────────────────┘                                    │📍│ │
│ ╰────────────────╯          ( Drag to pan · scroll to zoom )      [⤢ − 60% + ?] ╰╯ │
└──────────────────────────────────────────────────────────────────────────────┘
```

- **Top bar, 44px, charcoal.** Wordmark, project name (editable, from `project.name`), a two-way switch Canvas | DESIGN.md, then on the right: agent presence pill, undo/redo, Preview, Export menu (keeps "Load the Hearth sample…" and "Start over (blank)…"), Skills. No per-tab tool strips in the bar any more; tools move to the rail and the contextual toolbar.
- **Left project panel, floating, 272px, collapsible to a 40px rail (⌘\).** Three tabs:
  - *Layers*: Web flows and pages, Mobile flows and screens, Diagram shapes. Same tree and affordances as today's Outline (rename inline, ★ start, ⚠ for failing checks, × delete, drag to reorder within a flow), grouped by channel instead of hidden behind a tab.
  - *Activity*: the agent feed. Every SSE frame with `source === 'mcp'` becomes an entry: who, what action, which node, when. Clicking an entry selects and zooms to the target. This replaces the three-second pill as the memory of what the agent did.
  - *Review*: today's `ReviewPanel` (pins by kind and status, checks) moved here, since review is project-wide, not per-selection. The open-issue badge in the top bar opens this tab.
  - Bottom: *Ask your agent* composer. Typing a request and pressing Enter copies a prompt with project context (project name, selected node, open issues, MCP URL) to the clipboard and shows "Copied. Paste it in Claude Code or Codex." Nothing is sent anywhere.
- **Right tool rail, floating, 44px wide, vertically centered.** Select (V), Hand (H) · Add web page (P), Add mobile screen (M) · Rectangle (R), Ellipse (O), Line (L), Arrow (A), Pen, Text (T) · Pin a note (C), Tidy layout (⇧T). Hover shows a tooltip with the shortcut. The drawing tools act directly on the canvas; shapes live in world coordinates like today's `draw.shapes`.
- **Inspector, floating, 292px, contextual.** Appears only when something is selected; slides in from the right next to the rail. Tabs Content / Design / Review, same panels as today (`ContentPanel`, `DesignPanel`, `EdgePanel`) with the breadcrumb header. Project tokens live under Design when nothing is selected (a small "Project" button in the top bar opens it).
- **Zoom pill, bottom right.** Fit (⌘0), −, percentage, +, help (?). The help popover holds the keyboard map (§11).
- **Hint strip, bottom center.** One line of contextual guidance ("Drag to pan · scroll to zoom", "Press Esc to leave Shop"). Disappears after the first few sessions (localStorage flag).
- **Preview / Play** stays a full-viewport stage (§8.7), unchanged in behavior, restyled: dark `#141311` stage, bottom pill with ← · name · ×.

### 4.2 The canvas

- **Surface.** `#F3F2EF` with a 24px dot grid at 1px `#D9D6CF`, fading at zoom < 35%. Pan with drag on empty space, Space+drag anywhere, two-finger scroll; zoom with pinch, ⌘+wheel, ⌘+/−, ⌘0 fit all, ⌘1 zoom to selection. Range 10% to 400%. Zoom is around the cursor. The transform is one `translate(x,y) scale(k)` on a world `div`, the same model `DrawEditor` already uses for SVG (web/index.html ~2528).
- **Artboards.** Each node is an artboard: web pages at their breakpoint width (1280/768/390) rendered by the existing `BlockView` renderer, mobile screens at 390×844 in a thin 1px frame with 18px radius (the heavy 24px bezel goes; it comes back only in Preview/Play). White fill, hairline border, soft shadow. Name label above the artboard with ★ for start nodes and the breakpoint; label stays readable at any zoom (counter-scaled, like Figma).
- **Flow groups.** One rounded dashed group per flow, labeled "Marketing · 2 pages · web". Groups are computed from the nodes they contain (bounding box plus padding), not stored. Click the label to select the flow (rename, delete, set start).
- **Positions.** Nodes get an optional `pos {x, y}` in world units. Missing means auto-placed by the current lane algorithm (`laneOrder`, index.html ~119). Dragging an artboard sets `pos` through a new `move_node` action; "Tidy layout" clears positions for a flow (`tidy_flow`). Auto-placed and placed nodes coexist: auto layout fills left to right after the last placed node in that group. Mobile flows start to the right of web flows; the diagram group sits below.
- **Connectors.** Edges render in one SVG overlay in world space. Orthogonal-aware cubic curves from the out port (right edge midpoint) to the in port (left edge midpoint); back edges route below the group as dotted; cross-flow edges stay vermillion, everything else is `#8E8A80` 1.5px. Labels are white pill chips at the midpoint; hovering an edge raises it and its label. Drag from a port to create an edge exactly as today (`startConnect`), with the inline label input. Edge selection opens `EdgePanel`.
- **Levels of detail.** Below 25% artboards render as flat cards with the name only (no block DOM), to keep pan cheap with many pages. Between 25% and 100% the real renderer in `thumb` mode. At 100%+ the `edit` mode.
- **Edit in place.** Click selects the artboard (contextual toolbar appears above it: breakpoint 1280/768/390 for web, Grid, Specs, ▶ Preview, Duplicate, ⋯). Double-click, Enter, or ⌘1 "enters" it: the canvas animates (220ms) to fit the artboard, other artboards dim to 60%, and block/child selection works as in today's `WebPage` / `StoryPage` (chip drag to reorder, dashed nested selection, hotspots). Esc leaves the artboard (restores the previous viewport). The Palette (block types / archetypes) becomes a "+ Add block" button in the contextual toolbar and in the inspector; dropping a block follows the current insert rules.
- **Filmstrip for mobile** goes away; screens are siblings on the canvas. Reorder within a flow by dragging artboards past each other when auto-placed, or via the Layers tree.
- **Pins** keep their four targets (page block, screen, board, draw). Board pins become canvas pins in world coordinates. The pin marker counter-scales so it stays 20px on screen.
- **Specs and grid overlays** are off by default for new projects (the sample sets them on today). They render inside the entered artboard only.
- **Empty project.** A centered card on the canvas: "Add a web page", "Add a mobile screen", "Load the Hearth sample", and "Set up an agent" (copies the install/attach prompt from the README). Disappears when the first node exists.

### 4.3 Visual system (tokens)

Defined once in `tailwind.config` and as CSS variables so both Tailwind classes and inline styles can use them. Replace the ~170 hard-coded hex values in the chrome with these.

| Token | Value | Use |
|---|---|---|
| `canvas` | `#F3F2EF` | the infinite surface |
| `dot` | `#D9D6CF` | grid dots |
| `panel` | `#FFFFFF` | floating panels, artboards |
| `hair` | `#E3E0D9` | borders |
| `ink` | `#1C1B19` | text, active controls |
| `muted` | `#7A766D` | secondary text, icons |
| `faint` | `#A8A49B` | tertiary text, placeholders |
| `bar` | `#1C1B19` | top bar |
| `accent` | `#D8432A` | selection ring, pins, cross-flow edges, one primary button |
| `accentSoft` | `#FBE8E3` | selected row background |
| `edge` | `#8E8A80` | connectors |
| `ok` / `warn` | `#2E7D4F` / `#B23A1E` | check pills |

- Radii: 12px panels, 8px controls and rows, 6px chips, 999 pills. Artboards 0px (web) and 18px (phone).
- Elevation: `0 1px 2px rgba(20,18,14,.06), 0 8px 24px rgba(20,18,14,.08)` for floating panels; artboards lighter.
- Type: Inter 13px UI, 11px uppercase `.08em` section labels, 12px secondary. Fraunces only for the wordmark and designed content. No new fonts.
- Controls: 30px high inputs, 32px icon buttons, 28px segmented buttons. Focus ring 2px `accent` at 2px offset. All hit targets at least 28px.
- Motion: 120ms for hover and tab underlines, 180ms for panel open/close, 220ms ease-out for zoom-to-artboard. `prefers-reduced-motion` turns the zoom animation into a jump.
- The warm serif identity survives in the wordmark and in the designs; the tools go neutral so the designs are the only warm thing on screen.

### 4.4 Keyboard map (delta from §11)

Keep every existing shortcut. Add: `H` hand, `⌘0` fit all, `⌘1` zoom to selection / enter artboard, `Esc` leave artboard then clear selection, `⌘\` toggle left panel, `⌘.` toggle inspector, `Shift+T` tidy layout, `C` pin tool. `Space` pans (already). Draw tool keys stay.

## 5. Model and server changes (all additive)

| Change | Where | Why |
|---|---|---|
| `Node.Pos *Point` (`{x, y}`), omitted when auto | `internal/studio/types.go` | manual artboard placement |
| `move_node {nodeId, x, y}` and `tidy_flow {flowId}` actions | `reduce.go` handlers map, `validate.go`, MCP tool list in `internal/mcp/mcp.go`, `docs/final-requeriments.md` §12.3 | drag to place, reset to auto |
| `SharedView.Entered *Ref` (node currently being edited in place) | `types.go`, `set_view` | agents can see and set which artboard a person is inside; replaces `view.view.web/story === 'page'` semantics while keeping those fields for compatibility |
| `project.canvas.view {x, y, zoom}` persisted with a debounced `set_canvas_view` | mirror of `set_draw_view` | reopening the studio returns to the same place; agents can `focus` the canvas |
| Activity: client keeps the last 200 `{rev, source, type, payload, at}` frames | client only, from the SSE stream | no server change; `source` is already in the frame |
| Default `showSpecs`, `showGrid` false; sample seed keeps them off | `seed.go`, `DefaultView` | clean first impression |

Undo stays per tab on the server. The client maps each action to its tab as it does now (`act(type, payload, {tab})`); ⌘Z undoes in the tab of the current selection, or the last acted tab, and the toast names it ("Undid in Web").

## 6. What is left to the builder

- Exact Bézier routing for connectors into placed (non-lane) nodes; the rule is "leave right, enter left, never cross an artboard if a simple bow avoids it".
- Hit-testing and port placement when artboards are scaled below 25%.
- Where exactly the collapsed left panel and the inspector sit on viewports under 1200px (stack the inspector over the rail is fine).
- Icon set: inline SVG, 16px, 1.6 stroke, round caps (the mockup has a starter set).

## 7. Plan (phases, each shippable)

**Phase 0 · Spec and groundwork (1 day)**
- Amend `docs/final-requeriments.md`: replace §8.1 to §8.4 with the shell and canvas above, drop "manual storyboard node positioning" from §19, add the new actions to §12.3, add the new keys to §11. Keep §17 rows, re-word the ones that mention Page/Storyboard.
- Add the token table to `tailwind.config` and `:root`, and a `web/README` note on the section layout of `index.html`.
- Add a tiny harness: `scripts/ui-shots.mjs` (puppeteer-core against a `serve --dev --sample` instance) that captures the main states, for before/after review. Not part of CI.

**Phase 1 · Canvas core (3 to 4 days)**
- New `Canvas` component replacing `StoryboardView`, `WebEditor`, `StoryEditor` and `DrawEditor` as the single editor for the Canvas switch; `DesignDoc` stays for the DESIGN.md switch.
- Viewport state and pan/zoom (lift `DrawEditor`'s wheel and pan logic into a `useViewport` hook), `useCanvasView` persistence.
- Artboards from nodes with levels of detail; flow groups; connectors with existing `geom` logic generalized to arbitrary positions; port drag; pins in world space.
- Server: `Node.Pos`, `move_node`, `tidy_flow`, `set_canvas_view`, `Entered`, defaults. Go tests for reduce/validate/undo of the new actions.
- Drawing tools act on the same canvas (shapes already carry world coordinates).

**Phase 2 · Shell (2 to 3 days)**
- Top bar, floating left panel with Layers / Activity / Review and the prompt composer, tool rail, contextual inspector, zoom pill, hint strip, empty-project card.
- Remove `Palette` as a panel; add "+ Add block" to the contextual toolbar and inspector.
- Skills drawer becomes a sheet from the top bar.

**Phase 3 · Edit in place (2 days)**
- Enter/leave artboard with animated viewport; dimming; block, child and hotspot editing reusing `WebPage` / `StoryPage` internals; specs and grid inside the entered artboard; contextual toolbar.
- Keyboard map additions; Esc semantics.

**Phase 4 · Polish and verification (1 to 2 days)**
- Replace hard-coded hex in the chrome with tokens; tooltips with shortcuts; focus rings; reduced motion.
- Re-run the §17/§18 walkthrough with the new selectors; update `docs/verification-report.md` and the website screenshots in `docs/index.html`.
- Performance check: Hearth sample at 60fps pan/zoom, interaction under 100ms, zero console errors.

Rough total: 9 to 12 working days for one builder, or about half that with the canvas and shell built in parallel after phase 0.

## 8. Verification

- `make test` passes (new Go tests for `move_node`, `tidy_flow`, `set_canvas_view`, defaults).
- CI's single-Babel-block and vendor checks pass; no new network hosts.
- Scripted walkthrough of every §17 row in the new UI, recorded in `docs/verification-report.md`.
- Manual: blank project shows the empty card; sample loads with all 9 nodes visible at fit; drag a page, reload, it stays; Tidy restores lanes; agent action appears in Activity within one frame; undo in Web does not touch Mobile.

## 9. Decisions made here (the user can overturn any of them)

1. **Neutral light chrome, warm content.** Tools go `#F3F2EF`/white/charcoal; the warm paper look stays only inside the designs. Alternative: keep the beige everywhere (less contrast between tool and artifact).
2. **One canvas for web, mobile and diagram** rather than one canvas per channel. Alternative: a Web | Mobile | Diagram switch in the top bar, simpler undo story, less "whole product at once".
3. **Manual placement allowed, auto by default, Tidy to reset.** This amends §8.2/§19. Alternative: pan/zoom over the fixed lane layout only.
4. **No model call in the binary.** The prompt composer copies to the clipboard. Alternative: nothing in the left panel about prompting at all.
5. **The vermillion accent stays** as the single brand color, used less. Alternative: a cool blue accent like Stitch, which would also mean recoloring pins and cross-flow edges and the website.
