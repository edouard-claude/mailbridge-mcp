package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerMailboxStatus(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("mailbox_status",
		mcp.WithDescription("Get status information about a mailbox folder: total messages and unseen count."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox name (default: INBOX)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
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

		mailbox := req.GetString("mailbox", "INBOX")

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		info, err := imappool.MailboxStatus(client, mailbox)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("status failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Mailbox: %s\nMessages: %d\nUnseen: %d",
			info.Mailbox, info.Messages, info.Unseen,
		)), nil
	})
}
