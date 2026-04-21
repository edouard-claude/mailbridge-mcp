package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRenameMailbox(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("rename_mailbox",
		mcp.WithDescription("Rename an existing mailbox folder."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("old_name",
			mcp.Description("Current name of the mailbox"),
			mcp.Required(),
		),
		mcp.WithString("new_name",
			mcp.Description("New name for the mailbox"),
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

		oldName := req.GetString("old_name", "")
		if oldName == "" {
			return mcp.NewToolResultError("old_name is required"), nil
		}
		newName := req.GetString("new_name", "")
		if newName == "" {
			return mcp.NewToolResultError("new_name is required"), nil
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		if err := imappool.RenameMailbox(client, oldName, newName); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rename failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Mailbox renamed from %q to %q.", oldName, newName)), nil
	})
}
