# Setting Up Yucca with Claude Code

## 1. Build Yucca

```bash
make build
```

## 2. Install the binary

Copy the `yucca` binary to a location in your PATH:

```bash
cp yucca /usr/local/bin/yucca
```

## 3. Configure Claude Code hooks

Add to your project's `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "yucca hook session-start"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "yucca hook session-end"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Read|Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "yucca hook pre-tool-use"
          }
        ]
      }
    ]
  }
}
```

## 4. Configure MCP server

Add to your project's `.claude/settings.json`:

```json
{
  "mcpServers": {
    "yucca": {
      "type": "stdio",
      "command": "yucca",
      "args": ["mcp", "serve"]
    }
  }
}
```

## 5. Start a session

Open Claude Code in your project. Yucca will:
1. Start the daemon automatically (SessionStart hook)
2. Export `YUCCA_SESSION_ID`, `YUCCA_DAEMON`, `YUCCA_PROJECT` to Bash env
3. Block direct Read/Write/Edit of protected files (.env, *.tfvars, etc.)
4. Expose `yucca_secret_request` MCP tool for Claude to request secrets

## 6. Usage

When Claude needs a secret:
- It calls `yucca_secret_request` via MCP
- Your browser opens the approval UI
- You enter/select the secret and choose a policy
- Claude uses `yucca exec -- <command>` to run with secrets injected
