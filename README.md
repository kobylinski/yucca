# yucca

**Local secret management for AI coding assistants.** A single Go binary that keeps secrets in RAM, scoped to the session — never on disk, never in the model's context. The agent can *use* secrets without ever *seeing* them.

## Why

When an AI coding agent needs a secret — an API key for a test, a deploy token, a database password — you're usually stuck choosing between bad options:

- paste it into the chat (now it's in the model's context and the logs),
- drop it in a `.env` the agent reads in plaintext, or
- break flow and run the command yourself.

yucca gives the agent a fourth way: reference secrets by name, and they're resolved at execution time and masked in output.

## How it works

- **Daemon** — a local process that holds secrets in RAM (and the OS keychain for persistence), scoped to the session.
- **MCP server** (`yucca mcp serve`) — exposes controlled tools so the agent can store, recall, and use secrets via `{{YUCCA:alias}}` placeholders. Values are substituted only at run time and masked in command output.
- **Hooks** — integrate with the agent's session lifecycle and block reads of protected files.
- **Approval surfaces** — a local web UI, a macOS menu-bar app, and a headless TUI console (`yucca tui`) where you approve a secret's use or enter a new value.
- **Protected files** — `.env` and friends are redacted to `{{YUCCA:alias}}` before the agent reads them, and rehydrated on write (locally or to a remote host over SSH).

## Quick start

```bash
# Homebrew (macOS / Linux):
brew install kobylinski/tap/yucca           # the CLI
brew install --cask kobylinski/tap/yucca    # optional: the macOS menu-bar app

# …or with Go:
go install github.com/kobylinski/yucca/cmd/yucca@latest

cd your-project
yucca init        # detect secret-bearing files, register the MCP server + hooks
```

Or grab a prebuilt binary (macOS/Linux · arm64/amd64) from the [releases page](https://github.com/kobylinski/yucca/releases) — the macOS menu-bar app ships there as a signed, notarized `.dmg`.

Open your agent (Claude Code, Codex, …) in the project — yucca starts its daemon and the agent gains the `yucca_*` tools. Manage secrets and approve requests in the local UI it opens, or run `yucca tui` for a headless approval console.

## Security model

- Secrets live in RAM and the OS keychain — never in plaintext on disk, never in the model's context.
- Every use of a secret can require explicit human approval (`ask_session` / `ask_always` policies).
- **Temporary entries** (`persist: false`) live only for the session and are erased when it ends — ideal for a throwaway test token.
- Command output is masked; protected files are redacted before the agent sees them, and temporary secrets are never written to files.

## Status

Early but functional, and used daily by its author. Expect rough edges.

## License

[MIT](LICENSE) © Marek Kobyliński
