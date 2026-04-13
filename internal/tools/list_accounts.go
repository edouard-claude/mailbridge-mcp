package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerListAccounts(s *server.MCPServer, cfg *config.Config) {
	tool := mcp.NewTool("list_accounts",
		mcp.WithDescription("List all configured email accounts with their ID, label, and email address. Use this first to discover available accounts before other operations."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var sb strings.Builder
		sb.WriteString("Configured accounts:\n")
		for _, acc := range cfg.Accounts {
			fmt.Fprintf(&sb, "- %s: %s (%s)\n", acc.ID, acc.Label, acc.Email)
		}
		return mcp.NewToolResultText(sb.String()), nil
	})
}
