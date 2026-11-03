# AgentCanvas

AgentCanvas is a local design studio in a single binary. One page lets you write a `DESIGN.md`, map connected web and mobile flows, edit web pages and phone screens, draw SVG diagrams, preview as a user, and pin review notes.

Any coding agent that speaks [MCP](https://modelcontextprotocol.io) can drive the same canvas you see in the browser. You and every connected agent edit one project, and each sees the other's changes live.

- **One binary, no runtime dependencies.** The React UI, fonts, and every asset it loads are embedded, so the studio works offline.
- **One project per directory.** State lives in `./.agentcanvas/` next to your code.
- **Agent-neutral MCP.** Streamable HTTP at `/mcp`, plus stdio through `agent-canvas attach` or `agent-canvas serve --stdio`.

Website and guide: **https://maxiplux.github.io/agentcanvas/** (source: [docs/index.html](docs/index.html)).

## Install

One line, no prompts, no `sudo`. Run it again at any time to update.

```sh
# macOS / Linux
curl -fsSL https://maxiplux.github.io/agentcanvas/install.sh | sh
```

```powershell
# Windows PowerShell
irm https://maxiplux.github.io/agentcanvas/install.ps1 | iex
```

The installer picks the right binary from the [latest release](https://github.com/maxiplux/agentcanvas/releases/latest), verifies it against `SHA256SUMS`, and installs it:

- **macOS/Linux:** into the folder of an existing `agent-canvas`, else `/usr/local/bin` if writable, else `~/.local/bin`.
- **Windows:** into `%LOCALAPPDATA%\Programs\agent-canvas`, added to your user `PATH`.

Set `AGENT_CANVAS_INSTALL_DIR` to choose the folder, or `AGENT_CANVAS_VERSION=v0.1.1` to pin a release.

To update from the CLI:

```sh
agent-canvas update --check   # is a newer release out?
agent-canvas update           # download, verify, and replace this binary
```

**For coding agents**, paste:

> Install AgentCanvas with `curl -fsSL https://maxiplux.github.io/agentcanvas/install.sh | sh` (Windows: `irm https://maxiplux.github.io/agentcanvas/install.ps1 | iex`), run `agent-canvas skills export agentcanvas --agent claude` in this project, start `agent-canvas` here, connect to the MCP URL it prints, and call `get_canvas` first.

The full guide is at **https://maxiplux.github.io/agentcanvas/**.

### Build from source

You need Go 1.25 or later and `make`. No Node or npm is needed: the UI is one HTML file and its assets are vendored in `web/vendor/`.

```sh
git clone https://github.com/maxiplux/agentcanvas.git agentcanvas
cd agentcanvas
make test          # go vet + go test
make build         # → dist/agent-canvas
make release       # → dist/agent-canvas-<os>-<arch>[.exe], six targets
```

You can also run `go install ./cmd/agent-canvas` from the clone to place the binary in `$(go env GOPATH)/bin`. Every build uses `CGO_ENABLED=0`, so cross-compiling needs nothing else.

## Quick start

Run the binary in the directory whose design you want to work on:

```sh
cd my-project
agent-canvas            # same as: agent-canvas serve
```

It prints three lines:

```text
UI:  http://127.0.0.1:47321/
MCP: http://127.0.0.1:47321/mcp
Any streamable-HTTP MCP client can connect to the MCP URL; stdio clients can run "agent-canvas attach".
```

Open the UI URL; `agent-canvas --open` does that for you. A new project starts from the **Hearth** sample: two web flows, two mobile flows, a seeded `DESIGN.md`, and a few deliberate design flaws to fix.

- The server binds `127.0.0.1` only. It starts at port 47321 and tries the next 20 ports. `--port N` uses exactly `N`.
- Running `serve` again for a project that already has a server prints that server's URLs and exits with code 3.
- Press Ctrl+C to stop. Pending saves are flushed before exit.

## Connect an agent

Any MCP client that supports **streamable HTTP** connects to the MCP URL. `GET /mcp.json` returns a ready-made generic config with the port the server actually bound:

```json
{ "mcpServers": { "agentcanvas": { "type": "http", "url": "http://127.0.0.1:47321/mcp" } } }
```

For clients that can only start a command (**stdio**), point them at `attach`. It proxies stdio to the server already running for the current directory:

```json
{ "mcpServers": { "agentcanvas": { "command": "agent-canvas", "args": ["attach"] } } }
```

`attach` takes `--project DIR` or `--url URL` to choose another server. To start the studio and speak MCP on stdin/stdout in one process, use `agent-canvas serve --stdio`.

Once connected, the agent should call `get_canvas` first; it is the agent's view of the screen. `GET /guide` (also the MCP resource `agentcanvas://guide`) lists every tool. Edits made by agents show up in open browsers immediately, with a small `Agent · …` note in the top bar.

## Skills

Skills are `SKILL.md` packages that teach agents a workflow. They are stored per user in your OS config directory, under `agent-canvas/skills/`. The built-in `agentcanvas` skill explains how to drive the studio.

```sh
agent-canvas skills list [--json]
agent-canvas skills install ./path/to/skill [--force]
agent-canvas skills remove NAME
agent-canvas skills export agentcanvas --agent claude            # → ./.claude/skills/agentcanvas
agent-canvas skills export agentcanvas --agent codex --global    # → ~/.codex/skills/agentcanvas
agent-canvas skills export agentcanvas --dir ./somewhere         # → ./somewhere/agentcanvas
```

A skill folder holds `SKILL.md` (with YAML front matter `name` and `description`) and `skill.json` (`name`, `version`, `description`). Installs are validated and all-or-nothing. The studio's **Skills** drawer shows installed skills read-only.

## Command reference

```text
agent-canvas [serve] [--project DIR] [--port N] [--open] [--stdio] [--dev]
agent-canvas attach [--project DIR | --url URL]
agent-canvas skills list [--json]
agent-canvas skills install PATH [--force]
agent-canvas skills remove NAME
agent-canvas skills export NAME (--dir PATH | --agent claude|codex [--global]) [--force]
agent-canvas update [--check] [--version TAG]
agent-canvas version
agent-canvas --help
```

Exit codes: `0` ok · `1` usage or runtime error · `2` no free port · `3` project already served by another process.

## Project files

| Path | Contents |
|---|---|
| `.agentcanvas/project.json` | The design: tokens, flows, pages, screens, edges, drawing, doc, pins |
| `.agentcanvas/ui.json` | Last tab, view, breakpoint, and toggles |
| `.agentcanvas/server.json` | Running server's port and URLs (removed on exit) |
| `.agentcanvas/exports/` | Files written by the MCP `export` tool with `write: true` |
| `DESIGN.md` | Written by *Export › Save DESIGN.md to project* or `export design-md write:true` |

If `project.json` is corrupt, it is renamed to `project.json.bad-<timestamp>` and the studio starts from the sample.

## Development

```sh
go run ./cmd/agent-canvas serve --dev --project /tmp/scratch
```

`--dev` serves React's development builds and reads `web/` from disk, so editing `web/index.html` only needs a browser reload. The UI is one hand-written file, `web/index.html`, with JSX compiled in the browser; there is no bundler.

| Target | What it does |
|---|---|
| `make test` | `go vet ./...` and `go test ./...` |
| `make build` | `dist/agent-canvas` for this machine |
| `make release` | Six binaries in `dist/` |
| `make vendor` | Re-download pinned assets listed in `web/vendor/VERSIONS` and verify their sha256 |
| `make vendor-verify` | Verify the vendored files on disk against `VERSIONS` |

Layout: `cmd/agent-canvas` (entry), `internal/studio` (data model, reducer, invariants, undo, persistence), `internal/server`, `internal/hub` (live updates), `internal/mcp`, `internal/skills`, `internal/export`, `internal/cli`, `web/` (UI and vendored assets), and `skills/agentcanvas` (built-in skill).

The full product spec is [docs/final-requeriments.md](docs/final-requeriments.md). The acceptance results are in [docs/verification-report.md](docs/verification-report.md).

## CI and releases

GitHub Actions workflows live in `.github/workflows/`:

- **CI** (`ci.yml`) runs on every push and pull request. It checks formatting, verifies the vendored asset checksums, compiles the UI's JSX, runs `make test`, cross-builds all six binaries, and uploads them as workflow artifacts.
- **Release** (`release.yml`) runs when you push a tag starting with `v`. It builds the six binaries with that version stamped in, writes `SHA256SUMS`, and publishes a GitHub release with all of them attached:

```sh
git tag v0.1.0
git push origin v0.1.0
```

You can also start the release workflow manually from the Actions tab with a version string.
