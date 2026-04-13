package tools

import (
	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAll registers all MCP tools on the server.
func RegisterAll(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	registerListAccounts(s, cfg)
	registerListMailboxes(s, cfg, pool)
	registerSearchEmails(s, cfg, pool)
	registerReadEmail(s, cfg, pool)
	registerSendEmail(s, cfg)
	registerReplyEmail(s, cfg, pool)
	registerMoveEmail(s, cfg, pool)
	registerMarkEmail(s, cfg, pool)
}
