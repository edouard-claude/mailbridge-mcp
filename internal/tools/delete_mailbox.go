package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDeleteMailbox(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("delete_mailbox",
		mcp.WithDescription("Delete an empty mailbox folder. The folder must be empty; use delete_email or move_email first to clear it."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("name",
			mcp.Description("Name of the mailbox to delete"),
			mcp.Required(),
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

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		if err := imappool.DeleteMailbox(client, name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete mailbox failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Mailbox %q deleted successfully.", name)), nil
	})
}
