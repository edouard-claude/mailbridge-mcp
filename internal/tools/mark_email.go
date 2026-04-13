package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerMarkEmail(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("mark_email",
		mcp.WithDescription("Change flags on an email: mark as read/unread, flag/unflag."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox containing the email (default: INBOX)"),
		),
		mcp.WithNumber("uid",
			mcp.Description("UID of the email"),
			mcp.Required(),
		),
		mcp.WithString("action",
			mcp.Description("Action to perform: read, unread, flag, unflag"),
			mcp.Required(),
			mcp.Enum("read", "unread", "flag", "unflag"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
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

		mailbox := req.GetString("mailbox", "INBOX")
		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		action := req.GetString("action", "")
		if action == "" {
			return mcp.NewToolResultError("action is required"), nil
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		if err := imappool.MarkEmail(client, mailbox, uint32(uid), action); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("mark failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Email UID %d marked as %s in %s.", uid, action, mailbox)), nil
	})
}
