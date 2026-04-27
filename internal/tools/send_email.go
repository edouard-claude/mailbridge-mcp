package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/auth"
	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	smtpsender "github.com/edouard-claude/mailbridge-mcp/internal/smtp"
	goimap "github.com/emersion/go-imap/v2"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSendEmail(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("send_email",
		mcp.WithDescription("Send a new email from a configured account. Supports plain text body, CC, BCC. Does NOT support attachments."),
		mcp.WithString("account_id",
			mcp.Description("Account to send from"),
			mcp.Required(),
		),
		mcp.WithString("to",
			mcp.Description("Comma-separated recipient email addresses"),
			mcp.Required(),
		),
		mcp.WithString("cc",
			mcp.Description("Comma-separated CC recipients"),
		),
		mcp.WithString("bcc",
			mcp.Description("Comma-separated BCC recipients"),
		),
		mcp.WithString("subject",
			mcp.Description("Email subject"),
			mcp.Required(),
		),
		mcp.WithString("body",
			mcp.Description("Plain text email body"),
			mcp.Required(),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
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

		toStr := req.GetString("to", "")
		if toStr == "" {
			return mcp.NewToolResultError("to is required"), nil
		}
		subject := req.GetString("subject", "")
		if subject == "" {
			return mcp.NewToolResultError("subject is required"), nil
		}
		body := req.GetString("body", "")
		if body == "" {
			return mcp.NewToolResultError("body is required"), nil
		}

		to := splitAndTrim(toStr)
		cc := splitAndTrim(req.GetString("cc", ""))
		bcc := splitAndTrim(req.GetString("bcc", ""))

		password, err := auth.GetPassword(acc.Auth)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get password for %s: %v", acc.Email, err)), nil
		}

		msg, err := smtpsender.Send(acc, password, to, cc, bcc, subject, body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("send failed: %v", err)), nil
		}

		// Copy to Sent folder via IMAP APPEND
		if client, err := pool.Get(accountID); err == nil {
			if sentMailbox, err := imappool.FindSentMailbox(client); err == nil {
				imappool.AppendMessage(client, sentMailbox, []goimap.Flag{goimap.FlagSeen}, msg)
			}
		}

		return mcp.NewToolResultText(fmt.Sprintf("Email sent successfully from %s to %s.", acc.Email, toStr)), nil
	})
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
