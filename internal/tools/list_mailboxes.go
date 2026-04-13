package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerListMailboxes(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("list_mailboxes",
		mcp.WithDescription("List all mailbox folders (INBOX, Sent, Drafts, etc.) for a given email account."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier from list_accounts (e.g. 'perso', 'pro')"),
			mcp.Required(),
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

		acc := cfg.Account(accountID)
		if acc == nil {
			return mcp.NewToolResultError(fmt.Sprintf("unknown account: %q", accountID)), nil
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed for %s: %v", accountID, err)), nil
		}

		names, err := imappool.ListMailboxes(client)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list mailboxes failed: %v", err)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Mailboxes for %s (%s):\n", acc.Label, acc.Email)
		for _, name := range names {
			fmt.Fprintf(&sb, "- %s\n", name)
		}
		return mcp.NewToolResultText(sb.String()), nil
	})
}
