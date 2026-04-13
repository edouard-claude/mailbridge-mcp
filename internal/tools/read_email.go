package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerReadEmail(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("read_email",
		mcp.WithDescription("Read the full content of an email by its UID. Returns headers (from, to, cc, date, subject) and body (plain text preferred, HTML stripped if no plain text). Lists attachments with filenames and sizes but does not download them."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox name (default: INBOX)"),
		),
		mcp.WithNumber("uid",
			mcp.Description("Email UID from search_emails results"),
			mcp.Required(),
		),
		mcp.WithNumber("max_body_chars",
			mcp.Description("Max characters to return for body (default: 10000)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := req.GetString("account_id", "")
		if accountID == "" {
			return mcp.NewToolResultError("account_id is required"), nil
		}

		if cfg.Account(accountID) == nil {
			return mcp.NewToolResultError(fmt.Sprintf("unknown account: %q", accountID)), nil
		}

		mailbox := req.GetString("mailbox", "INBOX")
		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		maxBodyChars := req.GetInt("max_body_chars", cfg.Defaults.BodyMaxChars)

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed for %s: %v", accountID, err)), nil
		}

		parsed, err := imappool.FetchEmail(client, mailbox, uint32(uid), maxBodyChars)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read email failed: %v", err)), nil
		}

		return mcp.NewToolResultText(imappool.FormatEmail(parsed)), nil
	})
}
