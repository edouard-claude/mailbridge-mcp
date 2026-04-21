package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDeleteEmail(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("delete_email",
		mcp.WithDescription("Permanently delete an email. For soft delete (move to Trash), use move_email instead."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithNumber("uid",
			mcp.Description("UID of the email to delete"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox containing the email (default: INBOX)"),
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

		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		mailbox := req.GetString("mailbox", "INBOX")

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		if err := imappool.DeleteEmail(client, mailbox, uint32(uid)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Email UID %d permanently deleted from %s.", uid, mailbox)), nil
	})
}
