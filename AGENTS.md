# mailbridge-mcp

MCP server for multi-account IMAP/SMTP email access via stdio.

## Commands

```bash
make build        # Build binary to ./mailbridge-mcp
make install      # Build + copy to /usr/local/bin (required for Claude Code to pick up changes)
./mailbridge-mcp setup   # Interactive config: add/update an account
./mailbridge-mcp serve   # Start MCP stdio server (used by Claude/Cursor)
```

**Critical:** After any code change, run `make install` — Claude Code loads the binary from `/usr/local/bin/mailbridge-mcp`, not from the repo.

## Config

- File: `~/.config/mailbridge/accounts.json`
- Passwords: macOS Keychain (service `mailbridge-mcp`, account = email address)
- `setup` replaces accounts by ID in place (same ID = update, new ID = append)

## Architecture

```
cmd/mailbridge-mcp/main.go   → entry point, serve/setup subcommands
internal/config/config.go    → config loading from ~/.config/mailbridge/accounts.json
internal/auth/               → macOS Keychain read/write
internal/imap/               → IMAP connection pool
internal/smtp/sender.go      → SMTP sender with extractEmail() to strip display names
internal/tools/              → MCP tool registration + implementations (8 tools)
```

## Gotchas

- **SMTP addresses:** IMAP can return display names like `'Edouard' <edouard@squirrel.fr>`. `extractEmail()` in `internal/smtp/sender.go` strips these before `RCPT TO`. Always use it when passing addresses to SMTP.
- **No tests yet:** `go test ./...` will pass trivially (no `_test.go` files).
- **macOS-only:** Keychain auth via `go-keyring` — won't work on Linux without D-Bus/Secret Service.
- **Config path mismatch:** README says `~/.config/mailbridge/` but CLAUDE.md says `~/.config/mailbridge-mcp/`. The real path is `~/.config/mailbridge/accounts.json` (see `config.Load()`).
- **IMAP pool:** Connections are pooled and must be closed via `defer pool.Close()` in `runServer()`.

## Claude Code MCP config

Defined in `~/.claude.json`:
```json
"mailbridge": {
  "type": "stdio",
  "command": "/usr/local/bin/mailbridge-mcp",
  "args": ["serve"]
}
```

If `/mcp` fails with "Failed to reconnect":
1. Verify binary: `echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}},"id":1}' | /usr/local/bin/mailbridge-mcp serve`
2. Must return JSON with `serverInfo`
3. If crash: check `go vet ./...` and `go build ./cmd/...`

## Dependencies

- `mcp-go` — MCP server framework
- `go-imap/v2` — IMAP client (beta)
- `go-message` — MIME parsing
- `go-keyring` — macOS Keychain
