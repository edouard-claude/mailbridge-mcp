# mailbridge-mcp

MCP server for multi-account IMAP/SMTP email access via stdio.

## Commands

```bash
make build        # Build binary to ./mailbridge-mcp
make install      # Build + copy to /usr/local/bin + codesign (required for Claude Code)
make clean        # Remove local binary
./mailbridge-mcp setup   # Interactive config: add/update an account
./mailbridge-mcp serve   # Start MCP stdio server (used by Claude/Cursor)
```

**Critical:** After any code change, run `make install` — Claude Code loads the binary from `/usr/local/bin/mailbridge-mcp`, not from the repo. The Makefile also runs `codesign --force --sign -` on the installed binary (required on macOS to avoid Gatekeeper prompts).

**Testing the binary manually:**
```bash
echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}},"id":1}' | /usr/local/bin/mailbridge-mcp serve
```
Must return JSON with `serverInfo`. If silent crash: check `go vet ./...` and `go build ./cmd/...`.

## Config

- **Location:** `~/.config/mailbridge/accounts.json` (confirmed in `config.Load()` at `internal/config/config.go:73`)
- **CLAUDE.md is wrong** — it says `~/.config/mailbridge-mcp/config.yaml`. The real path is `~/.config/mailbridge/accounts.json`.
- Passwords: macOS Keychain (service `mailbridge-mcp`, account = email address). Also supports `env` auth type via `AuthConfig.EnvVariable`.
- `setup` replaces accounts by ID in place (same ID = update, new ID = append).
- Config is JSON (not YAML), with structure:

```go
type Config struct {
    Accounts []Account `json:"accounts"`
    Defaults Defaults  `json:"defaults"`
}
type Account struct {
    ID, Label, Email string
    IMAP  IMAPConfig  // Host, Port, TLS
    SMTP  SMTPConfig  // Host, Port, StartTLS, TLS
    Auth  AuthConfig  // Type (keychain|env), KeychainService, KeychainAccount, EnvVariable
}
type Defaults struct {
    MaxFetch       int  // default 50
    BodyMaxChars   int  // default 10000
    TimeoutSeconds int  // default 30
}
```

`Load()` auto-fills defaults if zero. Requires at least one account. Validates that every account has `id` and `email`.

## Architecture

```
cmd/mailbridge-mcp/main.go     → entry point: "serve" or "setup" subcommands
internal/config/config.go      → config loading/saving, types, Account() lookup
internal/auth/keychain.go      → GetPassword() (keychain or env), SetPassword()
internal/imap/client.go        → Pool: one IMAP connection per account, lazy connect with NOOP health check
internal/imap/search.go        → Search() with IMAP SEARCH criteria, EmailSummary, FormatSummaries()
internal/imap/fetch.go         → FetchEmail(), ParsedEmail, MIME body parsing, FormatEmail()
internal/imap/operations.go    → MoveEmail, MarkEmail, DeleteEmail, CopyEmail, ListMailboxes,
                                  CreateMailbox, DeleteMailbox, RenameMailbox, AppendMessage,
                                  FindSentMailbox, MailboxStatus
internal/smtp/sender.go        → Send(), SendReply(), BuildMessage() — RFC 5322 compliant MIME construction
internal/tools/registry.go     → RegisterAll() calls each register* function
internal/tools/*.go            → One file per MCP tool, each with a registerXxx() function
```

**Control flow:**
1. `main.runServer()` loads config → creates IMAP pool → creates MCP server → registers all tools → runs `server.ServeStdio()`
2. Each tool handler: validates params → gets account from config → gets IMAP client from pool → does IMAP/SMTP work → returns `mcp.NewToolResultText()` or `mcp.NewToolResultError()`
3. IMAP pool: `pool.Get(accountID)` checks if connection exists and is healthy (NOOP), reconnects if dead

## MCP Tool Pattern

Every tool follows this exact pattern (see `list_accounts.go` as minimal example):

```go
func registerXxx(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
    tool := mcp.NewTool("snake_case_name",
        mcp.WithDescription("..."),
        mcp.WithString("account_id", mcp.Description("..."), mcp.Required()),
        // ... more params ...
        // Always include these 4 annotations:
        mcp.WithReadOnlyHintAnnotation(true/false),
        mcp.WithDestructiveHintAnnotation(true/false),
        mcp.WithIdempotentHintAnnotation(true/false),
        mcp.WithOpenWorldHintAnnotation(true/false),
    )
    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // 1. Validate account_id
        // 2. Get account from cfg
        // 3. Validate other required params
        // 4. Get IMAP client from pool (for IMAP ops)
        // 5. Get password from auth (for SMTP ops)
        // 6. Do work
        // 7. Return result or error
    })
}
```

Key conventions:
- **Tool names:** `snake_case` (e.g., `search_emails`, `list_accounts`)
- **Params:** use `req.GetString()`, `req.GetInt()`, `req.GetBool()` with defaults
- **account_id** is always the first param and always required
- **mailbox** defaults to `"INBOX"` unless specified
- Always check `cfg.Account(accountID) == nil` before using the account
- **IMAP tools** use `pool.Get(accountID)` to get client; **SMTP tools** use `auth.GetPassword(acc.Auth)` then `smtpsender.Send()/SendReply()`
- Tools that send email (send_email, reply_email, send_draft) also copy the sent message to the Sent folder via `imappool.FindSentMailbox()` + `imappool.AppendMessage()`
- Error messages include the operation name, UID, mailbox, or account for debugging

## Complete Tool List (17 tools)

**Read (6):** `list_accounts`, `list_mailboxes`, `search_emails`, `read_email`, `mailbox_status`
**Write — email (4):** `send_email`, `reply_email`, `save_draft`, `send_draft`
**Write — mailbox (4):** `move_email`, `copy_email`, `mark_email`, `delete_email`
**Write — folder (3):** `create_mailbox`, `rename_mailbox`, `delete_mailbox`

Shared helper: `splitAndTrim()` in `send_email.go` — splits comma-separated strings and trims whitespace.

## IMAP Patterns

### Search (`search.go`)
- Uses `imap.SearchCriteria` with `Header` fields for FROM/SUBJECT
- Default `Since`: 7 days ago (avoids fetching entire mailbox)
- Results are reversed (newest first) and limited
- After search, does a FETCH for envelopes+flags to build `EmailSummary`
- Sorts by UID descending with insertion sort (small N)

### Fetch (`fetch.go`)
- `FetchEmail()`: SELECT mailbox → FETCH by UID with envelope+flags+body → parse MIME
- `parseBody()`: uses `go-message/mail.CreateReader()` — prefers `text/plain`, extracts `In-Reply-To`/`References` from MIME headers, lists attachments
- `FormatEmail()`: text format with headers, body, and attachment list
- `truncate()`: appends `"\n... [truncated]"` when body exceeds `maxBodyChars`
- `formatAddress()`: `Name <mailbox@host>` if name present, otherwise bare email

### Operations (`operations.go`)
- All operations: SELECT mailbox first, then act by UID
- `MarkEmail()`: `read`/`unread`/`flag`/`unflag` actions via STORE flags (silent)
- `DeleteEmail()`: STORE +Deleted → UID EXPUNGE (permanent, two-step)
- `FindSentMailbox()`: RFC 6154 SPECIAL-USE `\Sent` attribute first, then fallback to path component matching "sent" or "sent messages" (case-insensitive)
- `MailboxStatus()`: only requests `NumMessages`/`NumUnseen` — explicitly avoids `NumDeleted` because many servers (OVH, Dovecot) return BAD for it

### Pool (`client.go`)
- Thread-safe with `sync.Mutex`
- On `Get()`: runs NOOP to check health; if dead, closes and reconnects
- `Close()`: logs out + closes all connections

## SMTP Patterns (`sender.go`)

- **`extractEmail(addr)`**: Uses `net/mail.ParseAddress()` to strip display names. **Always use before `RCPT TO`** — SMTP only accepts bare addresses. Has a manual angle-bracket fallback.
- **`Send()` / `SendReply()`**: Build message → send via TLS or StartTLS → return raw message bytes for IMAP Sent copy
- **`BuildMessage()`**: RFC 5322 compliant MIME message with Date, From, To, Cc, Message-ID (crypto-random hex), Subject (RFC 2047 Q-encoding for non-ASCII), MIME-Version, Content-Type, Content-Transfer-Encoding (quoted-printable for non-ASCII body)
- Two send paths: `sendStartTLS()` (port 587) and `sendTLS()` (direct TLS, port 465)
- Auth: PLAIN auth with `smtp.PlainAuth`
- Header folding: `foldHeader()` wraps long headers (like References) to ≤78 chars per RFC 5322 §2.2.3
- Header sanitization: `sanitizeHeaderValue()` strips CR/LF to prevent header injection

## Threading/Reply (`reply_email.go`)

- `ensureAngleBrackets()`: normalizes Message-IDs to `<id@host>` form (IMAP servers return them with or without brackets)
- References chain: `References = original.References + " " + original.MessageID` (RFC 2822 §3.6.4)
- Subject: auto-prefixes with "Re: " if not already present
- Reply All: adds original To/Cc recipients who aren't the sender (uses `isSameEmail()` to compare)
- `isSameEmail()`: compares potentially formatted address against plain email, case-insensitive

## Gotchas

- **SMTP addresses:** IMAP can return display names like `'Edouard' <edouard@squirrel.fr>`. `extractEmail()` in `internal/smtp/sender.go` strips these before `RCPT TO`. Always use it when passing addresses to SMTP.
- **No tests:** `go test ./...` passes trivially (no `_test.go` files exist).
- **macOS-only:** Keychain auth via `go-keyring` — won't work on Linux without D-Bus/Secret Service. `env` auth type is available as alternative.
- **Config path confusion:** CLAUDE.md says `~/.config/mailbridge-mcp/config.yaml` — this is **wrong**. The real path is `~/.config/mailbridge/accounts.json` (JSON, not YAML). Trust the code in `config.go:73`, not CLAUDE.md.
- **IMAP pool:** Connections are pooled lazily. Must call `defer pool.Close()` after `NewPool()`.
- **goreleaser:** darwin-only builds (amd64 + arm64). CGO_ENABLED=0. Homebrew cask published to `edouard-claude/homebrew-tap`.
- **Message-IDs:** `go-imap` envelope returns Message-IDs with or without angle brackets depending on the server. Always use `ensureAngleBrackets()` from `reply_email.go` before emitting them in headers.
- **Mailbox Status:** Never request `NumDeleted` — causes BAD responses on many servers (OVH, Dovecot without IMAP4rev2).

## Dependencies

| Package | Purpose |
|---------|---------|
| `mcp-go` (mark3labs) | MCP server framework — `server.MCPServer`, `mcp.NewTool`, `mcp.CallToolRequest` |
| `go-imap/v2` (emersion) | IMAP client (beta) — `imapclient.Client`, `imap.SearchCriteria`, `imap.UIDSet` |
| `go-message` (emersion) | MIME parsing — `mail.CreateReader()` for body/headers/attachments |
| `go-keyring` (zalando) | macOS Keychain access — `keyring.Get()` / `keyring.Set()` |
