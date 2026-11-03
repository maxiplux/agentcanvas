# AgentCanvas — verification report

Build: `make test` (go vet + go test, all packages green) and `make release`. Walked on 2026-11-02 with fresh seed projects. Rows 1–33 were re-run in one scripted Chrome session against the production binary, resetting to the seed before each row; rows 38, 41 and 44 against the same server with a second window and the CLI acting from outside the page. The script drives the real UI (clicks, pointer drags, key events, inspector inputs) and reads the result from the DOM and `/api/state`.

## 1. Acceptance criteria (§17)

| # | Result | Evidence |
|---|---|---|
| 1 | Pass | Final browser run on a reset seed: blocks in order nav, hero, features, quote, footer; nav/features/footer backgrounds and quote ink equal the token hexes; typing P→Po→Pot→Pots in Content updated the canvas after every keystroke. |
| 2 | Pass | Bezel shows sc_welcome; 3 progress segments; filmstrip exactly Welcome, Pick your craft, Notifications. |
| 3 | Pass | 5 seed shapes rendered; a Rect drag committed x 608 y 456 w 72 h 48 (all multiples of 8). |
| 4 | Pass | Textarea equals the seed markdown; preview `##` = Intent, Users, Principles, Decisions; rail lists 5 headings. |
| 5 | Pass | Web Outline lists Marketing (Home, Shop) and Checkout (Cart, Confirm); story Outline lists Onboarding (Welcome, Pick your craft, Notifications) and Account (Home, Profile). |
| 6 | Pass | Two lanes (Marketing · 2 pages, Checkout · 2 pages), 4 thumbnails, 12 distinct curves ed_w1…ed_w12, 9 vermillion; Storyboard → Page returns to Home. |
| 7 | Pass | Port drag Shop → Cart + "Buy now" ↵ → edge kind cross, stroke #D8432A; Edge panel shows one disabled, checked radio and "kind is always cross (read-only)"; after giving Shop's nav link that label and target, Preview from Shop followed it to Cart. |
| 8 | Pass | sc_welcome has two outbound edges with pills at different positions; clicking each pill opens the Edge panel reading "Get started" and "Skip". |
| 9 | Pass | Play: Welcome → Pick your craft → Notifications → "Allow and finish" → sc_home (Account); progress bars switch to 2 segments. |
| 10 | Pass | + Flow → rename Support → + page (2 nodes) → delete flow: 9 nodes, 20 edges, 2 pins, 0 orphans. After deleting Checkout, Marketing's delete is disabled ("A channel must keep at least one flow"); the server also refuses it. |
| 11 | Pass | Project tokens accent → #0055FF: hero CTA and mobile CTA rgb(0,85,255); pins rgb(216,67,42); storyboard cross edge #D8432A. |
| 12 | Pass | Hero Design: typeRole display, weight 600, align left, pad 8, gap 4, 6 fill + 6 ink swatches; pad 32 → padding-top 32px. |
| 13 | Pass | Edit, blur, ⌘Z restores, ⌘⇧Z re-applies. Separate browser run: 51 inspector edits → exactly 50 ⌘Z steps (oldest "Edit 1"), undo button disabled, 51st ⌘Z refused; a Story edit + ⌘Z leaves the Web headline and Web redo stack unchanged. |
| 14 | Pass | Preview is a fixed full-viewport stage over all chrome; nav "Cart" shows pg_cart; Esc returns with Home current. |
| 15 | Pass | 390: Menu button, Features 1 column, headline 32px/38px, hero padding 16px; 768: 2 columns, headline 32px/38px. |
| 16 | Pass | Outline Home › Nav › Cart: only ln_home_cart outlined (dashed); inspector label "Cart", Target "Checkout › Cart". |
| 17 | Pass | Clicking the second feature card outlines only ft_2 (dashed); editing its title in Content updates the card. |
| 18 | Pass | Specs labels at 1280: 8, 24, 52/56, 12/16, gap 4; at 390: 16, 24, 32/38, 12/16, gap 4. |
| 19 | Pass | 12 overlay columns; a click on the quote through the grid selected bk_home_quote. |
| 20 | Pass | Alt-drag rect x 803 y 451 w 47 h 49 (not snapped); the same drag without Alt snaps (row 3). |
| 21 | Pass | Before: Outline ⚠ on CTA band, Design "Button (sm) 4.4:1 ⚠ Fails AA"; buttonInk → ink: "13.4:1 Passes AA" and the ⚠ is gone. |
| 22 | Pass | Before: Profile ⚠, Tap targets "Sign out: 36px (min 44)"; size md: "Sign out: 44px OK" and the ⚠ is gone. |
| 23 | Pass | Esc clears selection and exits Play; Delete removes (the duplicate); ⌘D duplicates; ⌘Z/⌘⇧Z (row 13, also inside the doc textarea); Space held → grab cursor and drag pans (view → 60,40) without touching shapes; ⌘0 resets view to 0,0,1; → follows the first primary edge (Welcome → craft → notify), ← goes back; "d", ⌘D and Backspace inside a text field leave the canvas unchanged. |
| 24 | Pass | Pins placed on the Home page (#3 issue, block target), sc_craft (#4 suggestion), the Cart storyboard thumbnail (#5 question, board target) and the draw canvas (#6 issue). |
| 25 | Pass | Kind picker offers exactly "! Issue", "✦ Suggestion", "? Question"; popover header "! Issue #3". |
| 26 | Pass | Click places + opens popover; Enter saves; Esc on an empty draft leaves 3 pins and pinCounter 3; drag moved #3 to (0.338, 0.425) and after reload it renders at (0.3383, 0.4254); Resolve turns #4 grey with ✓; popover Delete removes #6. |
| 27 | Pass | Filter Question + Open shows exactly one row (the open question); clicking it switches to Storyboard (view board) and selects the pin. |
| 28 | Pass | Badge 2 on the reset seed, 1 after resolving pin 1 from its popover; with pins shown, selecting the hero and editing its eyebrow still works. |
| 29 | Pass | After reload all 5 remaining pins are present. Exported hearth-home.html and hearth-draw.svg contain no pin markup; exported DESIGN.md lists pins 1 and 2 under Open issues. |
| 30 | Pass | On the seed the UI shows hero bodyRole xs and pad 8 (index 2); bk_shop_cta button 4.4:1 fails; sc_profile CTA is 36 px (sm). |
| 31 | Pass | Seed web storyboard: 12 curves ed_w1…ed_w12; pin 1 on the Home thumbnail and pin 2 on the Shop thumbnail. |
| 32 | Pass | Seed mobile storyboard: Onboarding · 3 screens, Account · 2 screens; ed_m4 #D8432A into the Account lane. |
| 33 | Pass | Doc textarea holds Intent, Users, Principles, Decisions and 8/8 sampled §7.5 sentences. |
| 34 | Pass | Separate headless Chrome 156 with all non-loopback traffic blocked (dead proxy + resolver that fails every host except 127.0.0.1; a fetch to unpkg.com from that browser fails): the production binary's UI loads fully — 9 resources, all from 127.0.0.1:47401; document.fonts reports Inter and Fraunces loaded; headline computed font Fraunces; screenshot shows the full studio. The interactive Chrome also showed loopback-only requests. |
| 35 | Pass | 47321 held → bound 47322 and printed UI URL, MCP URL, agent note; 47321–47341 held → exit 2. |
| 36 | Pass | Empty dir A → A/.agentcanvas/project.json with the seed; B independent; `--project A` from B while A runs → A's URLs, exit 3; restart restores project, tab (story) and breakpoint (tablet). |
| 37 | Pass | go-sdk v1.8.0 client program over StreamableClientTransport and over CommandTransport running `agent-canvas attach`: 42 tools, §13.2 instructions, get_canvas;  raw curl initialize/tools/list/get_canvas with only Content-Type + Accept; `serve --stdio` answers on stdio; attach edit shows in the browser; attach without a server exits 1 with the hint. |
| 38 | Pass | MCP update_block (sent from outside the browser) showed on the canvas with pill `Agent · Set Home › Hero headline to "Pots for every table"`; latency measured at 15 ms in an in-page MCP call; pill clears after 3 s. Window B followed an edit typed in window A; MCP get_node returned it; MCP undo {tab: web} reverted it in both windows. |
| 39 | Pass | get_canvas on the seed: 4 flows, 9 nodes, 20 edges, pins 1–2, contrast failure bk_shop_cta (4.4), tap "Profile › Sign out: 36px (min 44)", skill agentcanvas; same-channel set_link_target → isError + Next, rev unchanged (0). |
| 40 | Pass | initialize returns the §13.2 instructions; /guide == agentcanvas://guide and lists all 42 tools; /mcp.json has the bound port; no vendor names in tool names/descriptions. |
| 41 | Pass | `skills install ./testdata/skills/good` with the drawer open: `good` appeared in the drawer 1.1 s later, in `skills list`, and in MCP skills_list with its body; Bad_Name, no-skill-md, mismatch refused with nothing left behind; remove works. |
| 42 | Pass | Empty config dir lists agentcanvas (built-in); removing it and installing a package named agentcanvas are refused (exit 1). |
| 43 | Pass | export --agent claude → <project>/.claude/skills/agentcanvas/{SKILL.md,skill.json}; --global → ~/.claude/skills (tested with a sandboxed HOME); --dir X → X/agentcanvas; second export without --force refused. |
| 44 | Pass | Drawer lists name, version, description and a built-in chip; opening agentcanvas renders its SKILL.md (53 elements); the only controls are "← Skills" and "×"; footer "Manage skills with: agent-canvas skills --help"; Esc closes. |

## 2. Walkthrough (§18)

| Step | Result |
|---|---|
| 1 Doc | Pass — principle 3 edited; exported DESIGN.md has the edit, Flows (Web and Mobile) and both open issues. |
| 2 Web Storyboard | Pass — "Buy now" cross edge by port drag; Shop's nav link relabelled "Buy now" and targeted to Cart (shares the edge); Preview from Shop → Cart. |
| 3 Web phone width | Pass — 390 reflow; Specs on; hero bodyRole md (17/26) and pad 24 (16 at phone); Shop contrast fixed via buttonInk; undo ×2 and redo ×2 step correctly. |
| 4 Mobile Storyboard | Pass — Onboarding → Home edge present; Play Welcome → … → Home; avatar hotspot → Profile; pin placed on the CTA; size md; pin resolved; no tap failures. |
| 5 Draw | Pass — rect, ellipse, arrow, text, pen; moves with/without Alt; wheel zoom 135%; SVG export viewBox fitted. |
| 6 Pins | Pass — Issue/Suggestion/Question on a page, a screen, a storyboard node and the SVG; resolve; badge and Review filters correct. |
| 7 Reload | Pass — project, tokens, graph, doc, pins, tab and breakpoint restore. |
| 8 Exports | Pass — opened from disk in Chrome: hearth-home.html (5 blocks, Fraunces, links #shop/#cart, no pins); hearth-web-flows.html shows one page at a time and follows Home → Cart → Confirm → Home; hearth-onboarding.html plays Welcome → Pick your craft → Notifications → Account › Home with hotspots; hearth-draw.svg opens with a fitted viewBox and arrow marker. No pin or studio-chrome markup in any file. |
| 9 Console | Pass — no errors and no React warnings (React development build via --dev). Only vendor warnings from the Tailwind play script and Babel standalone, plus ERR_CONNECTION_REFUSED during the deliberate server stop. |
| 10 Agent | Pass — via `attach`: get_canvas, update_block buttonInk, update_pin resolved, export design-md write:true → DESIGN.md lists only pin 1. |
| 11 Two browsers | Pass — an edit in one window appears in the other. |
| 12 Restart | Pass — SIGINT removes server.json; restart recreates it and restores project and view; open pages reconnect without reload. |
| 13 Offline | Pass — headless Chrome with all non-loopback traffic blocked (verified: external fetch fails) loads the full studio with Inter and Fraunces; every request goes to 127.0.0.1. |
| 14 Skills | Pass — CLI install shows in the open drawer; built-in exported with --agent claude; test skill removed. |
| 15 Raw MCP | Pass — transcript below. |

### Raw MCP transcript (fresh seed, only `Content-Type` and `Accept` headers; long responses truncated)

```text
$ curl -s -X POST http://127.0.0.1:47395/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"8"}}}'
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"logging":{},"resources":{"listChanged":true},"tools":{"listChanged":true}},"instructions":"Drive the AgentCanvas studio only through these tools. Call `get_canvas` before editing. Read `skills_list` before inventing a workflow. Trust each tool result's `summary` and `changedIds` for what changed; do not assume.","protocolVersion":"2025-06-18","serverInfo":{"name":"ag … [truncated, 477 bytes]

$ curl -s -X POST http://127.0.0.1:47395/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'
(202 Accepted, empty body)


$ curl -s -X POST http://127.0.0.1:47395/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
{"jsonrpc":"2.0","id":2,"result":{"ttlMs":0,"cacheScope":"public","tools":[{"description":"Add a numbered review pin (issue, suggestion, question) on a page or screen (target node, optional blockId; x,y fractions 0..1), a storyboard thumb … [truncated, 25817 bytes]

$ curl -s -X POST http://127.0.0.1:47395/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_canvas","arguments":{}}}'
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"{\"rev\":0,\"view\":{\"tab\":\"web\",\"view\":{\"web\":\"page\",\"story\":\"page\"},\"breakpoint\":\"desktop\",\"currentNodeId\":{\"web\":\"pg_home\",\"mobile\":\"sc_welcome\"},\"selection\":null,\"preview\":false,\"showPins\":true,\"showGrid\":false,\"showSpecs\":false},\"history\":{\"doc\":{\"canUndo\":false,\"canRedo\":false},\"draw\":{\"canUndo\" … [truncated, 10477 bytes]

$ curl -s -X POST http://127.0.0.1:47395/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"set_link_target","arguments":{"nodeId":"pg_home","owner":{"blockId":"bk_home_nav","childId":"ln_home_story"},"toId":"sc_home"}}}'
{"jsonrpc":"2.0","id":4,"result":{"content":[{"type":"text","text":"Refused: edges must connect nodes in the same channel (pg_home is web, sc_home is mobile). Next: call get_canvas and pick a web node as toId."}],"isError":true}}

$ curl -s -X POST http://127.0.0.1:47395/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"agentcanvas://guide"}}'
{"jsonrpc":"2.0","id":5,"result":{"ttlMs":0,"cacheScope":"public","contents":[{"uri":"agentcanvas://guide","mimeType":"text/markdown","text":"# AgentCanvas\n\nAgentCanvas is a local design studio for web pages, mobile story flows, connected storyboards, an SVG drawing, a `DESIGN.md`, and numbered review pins. One process serves one project; the designer's browser and every connected agent edit the same canvas and see … [truncated, 9373 bytes]
```

Summary of that run: initialize instructions = §13.2 text; 42 tools; get_canvas 4 flows / 9 nodes / 20 edges / pins 1–2 / contrast `bk_shop_cta` 4.4 / tap "Profile › Sign out: 36px (min 44)" / skills `agentcanvas`; the same-channel refusal is `isError` and rev stays 0.

## 3. `// SPEC:` decisions

| Where | Decision |
|---|---|
| `internal/studio/graph.go` | §0 conflict rule ("a table of values beats prose"): §7.1 lists ed_w11 (Confirm → Cart, both Checkout) as cross; the table wins, so a stored same-flow cross is tolerated; kinds are recomputed on every create/retarget. |
| `internal/studio/graph.go` | Two links with the same (node, toId, label) share one edge; it is removed only when no link owns it. |
| `internal/studio/algo.go` | Body contrast role: hero bodyRole, feature cards sm, CTA band body md, else typeRole. |
| `internal/studio/algo.go + web/index.html` | Mobile CTA labels render at lg weight 600 (large text), so the CTA check uses 3.0 — keeps "one low-contrast CTA" in the seed. |
| `internal/studio/reduce.go` | Token edits from draw/doc use the web history. |
| `internal/studio/reduce.go` | Selecting a pin also switches tab/view/node to show it. |
| `internal/studio/store.go` | Skills changes bump rev so browsers accept the new list; reset_project returns pinCounter to 2. |
| `internal/server/server.go` | POST /api/export/{kind}/write backs "Save DESIGN.md to project" (§12.2 lists no write endpoint). |
| `internal/mcp/mcp.go` | jsonschema-go (the go-sdk's own schema package) imported for enum schemas; Stateless + JSONResponse so plain curl gets application/json; relative skills_install paths resolve against the project dir. |
| `internal/skills/skills.go` | AGENT_CANVAS_SKILLS_DIR overrides the skills folder (tests never touch the real config dir). |
| `web/index.html` | §9.3 edge hit test uses a 12 px transparent stroke (same 6 px tolerance); text shape (x, y) is the baseline, like the SVG export. |

## 4. Known deviations

- `ed_w11` (confirmed by the project owner on 2026-11-02: keep `cross`) follows the spec's own conflict rule. §0 says: "When two sections seem to conflict, the more specific one wins (a table of values beats prose)." So it is not a deviation: the §7.1 table and §17 row 6 (9 vermillion) both say cross, while the §6.1 prose implies primary (both pages are in Checkout). The seed keeps the table value; the reducer still applies §6.1 on every create/retarget (Go test `TestEdW11RecomputedOnRetarget`: relabel/retarget makes it primary, a requested cross on a same-flow edge is refused, new same-flow edges are primary).

- Console: the vendored Tailwind play script and Babel standalone each log one console.warn on load ("should not be used in production"). They are not errors or React warnings, but the console is not silent.
- Seed content the spec leaves open was filled in: screen titles/bodies, feature and shop card bodies, the pen loop points, and per-type defaults for inserted blocks/screens.
- MCP `export` returns two text items (a "Wrote …/Exported …" line, then the file). `upsert_flow` over MCP cannot send `startId: null` (optional only).
- Storyboard: leftward and back edges bow through the channel between/below lanes, and node pins are also drawn on thumbnails (needed for §17 row 31). The board has a 96 px left margin for curves.
- The 1280 frame scrolls horizontally when the canvas is narrower than the breakpoint (no zoom-to-fit). The 390×844 phone is shown 1:1 and the canvas scrolls to the filmstrip.
- Draw "drag select" is click-select plus drag-move (single selection). Draw adds V/R/O/L/A/T/P/H tool keys.
- Page export phone media query is literally max-width 390px; mobile and web node slugs are unique per channel.
- go.mod says go 1.25.0 because go-sdk v1.8.0 requires it (satisfies "1.24 or later").

## 5. Sizes, browsers, clients

| Binary (CGO_ENABLED=0, -s -w) | Size |
|---|---|
| darwin-amd64 | 14.9 MB |
| darwin-arm64 | 14.2 MB |
| linux-amd64 | 14.6 MB |
| linux-arm64 | 13.9 MB |
| windows-amd64.exe | 14.9 MB |
| windows-arm64.exe | 14.1 MB |

- `web/index.html`: 163 KB, 2,876 lines (one Babel script, eight banner sections).
- Browser: Chrome 156 on macOS (driven through Chrome DevTools), with both `serve --dev` (React development builds) and the embedded production build.
- MCP clients: go-sdk v1.8.0 client over streamable HTTP and through `attach`, raw `curl`, newline-delimited JSON-RPC over `serve --stdio` and `attach`, and in-page `fetch` to `/mcp`.
