# Yucca

Local secret management for AI coding assistants. A single Go binary (`yucca`) keeps secrets in RAM, scoped to the session — never on disk, never in the model's context. The agent *uses* secrets (via `{{YUCCA:alias}}` placeholders) without ever *seeing* them.

See [README.md](README.md) for the user-facing overview and [docs/guides/claude-code-setup.md](docs/guides/claude-code-setup.md) for wiring it into Claude Code.

## Architecture

One `yucca` binary, several roles:

- **Daemon** (`yucca daemon`) — long-running local process on `http://127.0.0.1:9777`; holds secrets in RAM + the OS keychain, owns the approval flow, serves the embedded web UI, emits WebSocket events. Installs as a launchd LaunchAgent (`co.kobylinski.yucca.daemon`) via `yucca daemon install`.
- **MCP server** (`yucca mcp serve`) — stdio JSON-RPC, spawned per agent session; exposes the `yucca_*` tools (secret store/request/capture, exec, protected-file read/write, notes, clipboard) and a process-local temp store. Talks to the daemon over HTTP.
- **Hooks** (`yucca hook …`) — Claude Code lifecycle: `SessionStart` injects context, `PreToolUse` **denies** direct Read/Grep/Bash access to protected files and redirects to `yucca_file`.
- **exec** (`yucca exec`) — runs a command with `{{YUCCA:alias}}` placeholders substituted at run time and secret values masked in the output.
- **Approval surfaces** — the embedded web UI, and a headless TUI console (`yucca tui`) for SSH/remote sessions.
- **macOS tray app** (`client/macos/`, Swift) — a menu-bar client that talks to the daemon; distributed separately as a signed/notarized `.app`.

## Project Structure

```
yucca/
├── cmd/yucca/          # CLI entry points (cobra)
├── internal/
│   ├── daemon/         # HTTP daemon on :9777 — sessions, approvals, WebSocket, embedded UI
│   ├── mcp/            # MCP server (stdio JSON-RPC) — the yucca_* tools + process-local temp store
│   ├── hook/           # Claude Code hooks (SessionStart / SessionEnd / PreToolUse deny)
│   ├── exec/           # Subprocess execution: {{YUCCA:alias}} substitution + output masking
│   ├── proxy/          # Protected-file read/write: redact to {{YUCCA:alias}}, rehydrate on write
│   ├── store/          # Keychain-backed credential + note store (keychain service "yucca")
│   ├── scanner/        # Secret pattern scanner/parser (used by init + file protection)
│   ├── init/           # `yucca init` — pattern detection, file/field selector, diff preview
│   ├── clipboard/      # OS clipboard (pbcopy/wl-copy/xclip); Read exported only for capture
│   ├── service/        # OS service install (launchd LaunchAgent / systemd unit)
│   ├── fuzzy/          # Fuzzy matching for cross-project secret search
│   ├── tui/            # Headless approval console (Bubble Tea) for SSH/remote sessions
│   └── ui/             # Embedded web-UI glue (go:embed)
├── ui/                 # SvelteKit source for the approval UI (built → internal/daemon/ui_dist)
├── client/macos/       # Swift menu-bar app (Yucca.app) + Makefile (build/sign/notarize/DMG)
├── homebrew/           # Formula + cask templates (CI fills version/sha → kobylinski/homebrew-tap)
├── docs/guides/        # Published guides
├── docs/journal/       # Local-only dated dev notes — GITIGNORED, not published
├── .github/workflows/  # ci.yml (build/test) + release.yml (GoReleaser + signed DMG + tap bump)
├── .goreleaser.yml     # Cross-platform binary release config
├── CLAUDE.md · README.md · LICENSE · go.mod · go.sum
```

## Development Commands

```bash
make install          # build UI (pnpm) → embed → go install ./cmd/yucca   (use after any change)
make build            # build UI → embed → local ./yucca binary (no install)
go test ./...         # tests
go run ./cmd/yucca daemon       # run the daemon (foreground)
go run ./cmd/yucca mcp serve    # run the MCP server (stdio)

make -C client/macos install    # build + ad-hoc sign Yucca.app → /Applications (local dev)

# release: tag v* → CI builds binaries + signed/notarized DMG + bumps the Homebrew tap
git tag vX.Y.Z && git push origin vX.Y.Z
```

The UI is embedded at build time (`internal/daemon/ui_dist`, gitignored) — always rebuild via `make` after touching `ui/`.

## Key Conventions

- **RAM-only secrets** — never written to disk; OS keychain (service `"yucca"`) for persistence; state under `~/.yucca`, per-project config in `.yucca.yaml`.
- **Placeholders** — reference secrets as `{{YUCCA:alias}}` everywhere (exec, files); values are substituted at run time, masked in output, never returned to the model.
- **Session-only temp entries** — `persist: false` on the `yucca_secret_store` / `yucca_note_store` (and `_capture`) tools makes an entry that lives in the MCP process only, is never written to disk, and cannot be rehydrated into files.
- **Protected files** — read/written only via `yucca_file` (local paths or `user@host:path` over SSH); direct reads are blocked by the PreToolUse hook.
- **Naming** — `co.kobylinski.*` reverse-DNS (app id `co.kobylinski.yucca`, daemon label `co.kobylinski.yucca.daemon`); Homebrew tap `kobylinski/tap`.
- **No cgo** — cross-compiles cleanly for darwin/linux × amd64/arm64 (keychain via the `security` CLI on macOS, D-Bus on Linux).
- **Stack** — cobra (CLI), stdlib `net/http` (daemon IPC), `go:embed` (UI assets), Bubble Tea (TUI), SvelteKit + Tailwind (web UI), Swift/AppKit (macOS tray).

## Documentation Storage

- `docs/guides/` — permanent, published guides.
- `docs/journal/YYYY-MM-DD/` — dated research / implementation / analysis notes. **Local-only (gitignored)** — keep working notes here, not in the published tree.
