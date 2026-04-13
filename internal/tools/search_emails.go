package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSearchEmails(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("search_emails",
		mcp.WithDescription("Search for emails in a specific account and mailbox. Returns a list of matching email summaries (subject, from, date, UID). Defaults to last 7 days. Use 'since' to widen the time window (e.g. '2025-01-01' for older emails)."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox name (default: INBOX)"),
		),
		mcp.WithString("query",
			mcp.Description("Free-text search query (searched in subject via IMAP SEARCH)"),
		),
		mcp.WithString("from",
			mcp.Description("Filter by sender email or name"),
		),
		mcp.WithString("since",
			mcp.Description("Emails since this date (YYYY-MM-DD). Defaults to 7 days ago. Set to an earlier date to search further back."),
		),
		mcp.WithString("before",
			mcp.Description("Emails before this date (ISO 8601: YYYY-MM-DD)"),
		),
		mcp.WithBoolean("unseen_only",
			mcp.Description("Only return unread emails"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results (default: 20, max: 50)"),
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
		query := req.GetString("query", "")
		from := req.GetString("from", "")
		since := req.GetString("since", "")
		before := req.GetString("before", "")
		unseenOnly := req.GetBool("unseen_only", false)
		limit := req.GetInt("limit", 20)
		if limit > 50 {
			limit = 50
		}
		if limit < 1 {
			limit = 20
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed for %s: %v", accountID, err)), nil
		}

		summaries, err := imappool.Search(client, mailbox, query, from, since, before, unseenOnly, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		return mcp.NewToolResultText(imappool.FormatSummaries(accountID, mailbox, summaries)), nil
	})
}
