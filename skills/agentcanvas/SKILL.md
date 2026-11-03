---
name: agentcanvas
description: Drive the AgentCanvas design studio over MCP: read the canvas, edit pages, screens, flows, drawings, DESIGN.md, and review pins.
---

# AgentCanvas

AgentCanvas is a local design studio: web pages, mobile story screens, connected storyboards, an SVG drawing, a `DESIGN.md`, and numbered review pins. One `agent-canvas` process serves one project (stored in `.agentcanvas/` of the project directory). The designer's browser and every connected agent edit the **same** canvas and see each other's changes live.

## When to use it

- The user asks to design, review, or improve a web page, a mobile flow, a storyboard, a diagram, or the project's `DESIGN.md`.
- The user mentions AgentCanvas, the studio, the canvas, pins, flows, or the design doc of a project that has a `.agentcanvas/` folder.

## Start or connect

- Not installed yet? Install or update in one line: `curl -fsSL https://maxiplux.github.io/agentcanvas/install.sh | sh` (Windows: `irm https://maxiplux.github.io/agentcanvas/install.ps1 | iex`). `agent-canvas update` updates an existing install.

- Set a project up once: `agent-canvas init` in the project directory. It creates `.agentcanvas/`, copies the skills to `.claude/skills/` and `.agents/skills/`, and adds an `agentcanvas` MCP server to `.mcp.json` and `.codex/config.toml` (command `agent-canvas attach --start`). If the AgentCanvas MCP tools are not available to you, run `agent-canvas init`, then ask the user to restart or reload the agent so it picks up the MCP server.
- **You can start and show the studio yourself.** `agent-canvas open` starts it in the background when it is not running and opens the UI in the user's browser; it prints the UI and MCP URLs and returns, and the studio keeps running. Use `agent-canvas open --no-browser` to only start it and get the URLs. Tell the user the UI URL so they can watch and edit with you.
- `agent-canvas attach --start` (what `init` configures) proxies MCP over stdio and starts the studio in the background first when none is running. Plain `agent-canvas attach` only connects to a studio that is already running.
- `agent-canvas serve` runs the studio in the foreground and prints the UI URL and the MCP URL (`http://127.0.0.1:<port>/mcp`). Any streamable-HTTP MCP client connects to the MCP URL; `GET /mcp.json` returns a generic client config. `agent-canvas serve --stdio` starts the studio and speaks MCP on stdin/stdout in one process.

## Always call `get_canvas` first

`get_canvas` is your screen. It returns the shared view (tab, view, breakpoint, current node, selection, toggles), history flags, tokens, flows with node ids in storyboard order, every node (blocks with their child ids, labels and targets; screens with CTA and hotspots), edges, doc headings, pins, failing checks, the open-issue count, and installed skills. Pass `full: true` for the complete project JSON. Use `get_node` for one page or screen in full.

## Tools

### Read

- `get_canvas` — the whole studio as JSON; call before editing.
- `get_node` — one node with its in/out edges and pins.
- `get_doc` — the `DESIGN.md` markdown.
- `skills_list` — installed skills with their `SKILL.md` bodies; read it before inventing a workflow.

### Navigate (shared with the designer's browser)

- `set_tab` — switch tab (`web`, `story`, `draw`, `doc`) and optionally `view` (`page` or `board`).
- `set_breakpoint` — `desktop` (1280), `tablet` (768), `phone` (390); checks are computed at this breakpoint.
- `set_selection` — select a node, block or child, hotspot, edge, shape, or pin (selecting a node makes it current).
- `set_toggles` — `showPins`, `showGrid`, `showSpecs`.
- `preview` — enter or leave Preview/Play.

### History

- `undo` / `redo` — per-tab history (`web`, `story`, `draw`, `doc`); default is the current tab. Shared with the designer.
- `reset_project` — restore the seed project; needs `confirm: true`.

### Edit

- `update_tokens` — token colors (hex) and type sizes; `space` is fixed.
- `upsert_flow`, `delete_flow` — create (with one empty node), rename, set start; delete with cascade.
- `upsert_node`, `reorder_nodes`, `delete_node` — pages and screens; delete cascades edges, pins, and link targets.
- `insert_block`, `update_block`, `upsert_child`, `delete_child`, `reorder_blocks`, `duplicate_block`, `delete_block` — web page blocks (nav, hero, features, quote, cta, footer) and their links or feature items.
- `update_screen`, `upsert_hotspot`, `delete_hotspot` — mobile screens, their CTA, and hotspots.
- `set_link_target` — the only way to point a link, CTA, or hotspot at a node; it writes the matching edge.
- `upsert_edge`, `delete_edge` — storyboard edges (branches with labels); deleting clears the owning link.
- `add_shape`, `update_shape`, `duplicate_shape`, `delete_shape` — SVG draw shapes (rect, ellipse, line, arrow, text, pen).
- `set_doc` — replace the `DESIGN.md` markdown.
- `add_pin`, `update_pin`, `delete_pin` — review pins (issue, suggestion, question); resolve with `update_pin {resolved: true}`.

### Output

- `export` — `page`, `web-flows`, `mobile-flow`, `draw-svg`, or `design-md`; `write: true` saves into the project (`DESIGN.md` at the root, others under `.agentcanvas/exports/`).

### Packages

- `skills_install`, `skills_remove` — manage SKILL.md packages (text only; they never add tools).

## The design loop

1. `get_canvas`: read `checks` (contrast and tap-target failures) and the open pins.
2. Fix one problem at a time with the edit tools (for example `update_block` on a CTA band's `buttonInk`, or `update_screen` with `cta: {size: "md"}`).
3. Read the result: `summary`, `changedIds`, `failingChecks`, and `openIssues` tell you what changed.
4. Resolve the pin you addressed with `update_pin {pinId, resolved: true}`.
5. Record decisions in `DESIGN.md` with `set_doc` when useful, then `export {kind: "design-md", write: true}`.

## Invariants that refuse actions

A refused action leaves the project unchanged and returns `Refused: <rule>. Next: <what to call>.` The common ones:

- **Same channel**: edges and link targets connect nodes of the same channel (web pages to web pages, mobile screens to mobile screens).
- **Link targets**: `toId` fields cannot be patched with `update_block` or `update_screen`; use `set_link_target` (pass `toId: null` to clear).
- **Last flow**: a channel always keeps at least one flow; create another flow before deleting the last one.
- **Cross kind is read-only**: an edge between different flows is always `cross`; same-flow edges are `primary`, `alt`, or `back`.
- **Space is an index**: `pad` and `gap` are indices 0–7 into `[0 4 8 16 24 32 48 64]`, not px. Type roles are `xs sm md lg xl display`; weights `400 500 600`; colors are token keys (`paper ink muted accent wash line`), never hex.
- **Unique edges**: at most one edge per (from, to, label); use a different label for a branch.
- **Pins need text**: an empty pin body is refused.

## Trust the results

`summary` and `changedIds` are the source of truth for what changed; do not assume. The designer may be editing at the same time: re-read with `get_canvas` or `get_node` before a multi-step change.
