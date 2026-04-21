package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerCopyEmail(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("copy_email",
		mcp.WithDescription("Copy an email to a different mailbox folder. The original email remains in the source mailbox."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithNumber("uid",
			mcp.Description("UID of the email to copy"),
			mcp.Required(),
		),
		mcp.WithString("from_mailbox",
			mcp.Description("Source mailbox (default: INBOX)"),
		),
		mcp.WithString("to_mailbox",
			mcp.Description("Destination mailbox (e.g. 'Archive', 'INBOX.Important')"),
			mcp.Required(),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := req.GetString("account_id", "")
		if accountID == "" {
			return mcp.NewToolResultError("account_id is required"), nil
		}
		if cfg.Account(accountID) == nil {
			return mcp.NewToolResultError(fmt.Sprintf("unknown account: %q", accountID)), nil
		}

		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		fromMailbox := req.GetString("from_mailbox", "INBOX")
		toMailbox := req.GetString("to_mailbox", "")
		if toMailbox == "" {
			return mcp.NewToolResultError("to_mailbox is required"), nil
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		if err := imappool.CopyEmail(client, fromMailbox, uint32(uid), toMailbox); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("copy failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Email UID %d copied from %s to %s.", uid, fromMailbox, toMailbox)), nil
	})
}
