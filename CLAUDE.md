# Yucca

Local secret management for AI coding assistants. Single Go binary that keeps secrets in RAM, scoped to session lifetime.

## Quick Links

- [Project Brief](docs/brief.md) - mission, problem, solution, architecture
- [Technology Stack](docs/stack.md) - Go, libraries, providers
- [MVP Use Cases](docs/mvp.md) - acceptance criteria and test scenarios

## Project Structure

```
yucca/
├── cmd/            # CLI entry points (cobra commands)
├── internal/
│   ├── daemon/     # HTTP daemon, sessions, WebSocket
│   ├── exec/       # Subprocess execution with secret injection + masking
│   ├── hook/       # Claude Code hook handlers (session, pre-tool-use)
│   ├── init/       # Project init with pattern detection, file/field selector
│   ├── mcp/        # MCP server (stdio JSON-RPC, secret_request + exec tools)
│   ├── proxy/      # Protected file read/write with placeholder redaction
│   ├── scanner/    # Secret pattern scanner/parser
│   ├── store/      # Keychain-backed credential store with metadata
│   ├── tui/        # Bubble Tea terminal UI
│   └── ui/         # Embedded HTML/JS approval UI
├── ui/             # HTML/JS/CSS source for approval UI (embedded at build)
├── docs/
│   ├── journal/    # Daily research & implementation notes
│   │   └── YYYY-MM-DD/
│   ├── guides/     # Permanent guides and references
│   ├── plans/      # Design specs and implementation plans
│   └── summaries/  # Project summaries and decisions
├── CLAUDE.md
├── go.mod
└── go.sum
```

## Documentation Storage

All research, plans, and analysis documents must be stored in organized directories:

```
docs/
├── journal/
│   └── YYYY-MM-DD/
│       ├── research_documents.md
│       ├── implementation_plans.md
│       └── analysis_reports.md
├── guides/
│   └── permanent_guides.md
└── summaries/
    └── project_summaries.md
```

## Development Commands

```bash
make install                          # Build UI + install Go binary (use after any change)
make build                            # Build UI + local binary (no go install)
go test ./...                         # Test
go run ./cmd/yucca daemon           # Run daemon
go run ./cmd/yucca mcp serve        # Run MCP server
```

## Key Conventions

- All secrets are RAM-only, never written to disk
- Protected files use `{{YUCCA:id}}` placeholder format
- HTTP on localhost for daemon IPC
- Cobra for CLI subcommands
- Stdlib `net/http` for local HTTP server
- `go:embed` for UI assets
