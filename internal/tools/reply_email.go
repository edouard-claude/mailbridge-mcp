package tools

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/auth"
	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	smtpsender "github.com/edouard-claude/mailbridge-mcp/internal/smtp"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerReplyEmail(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("reply_email",
		mcp.WithDescription("Reply to an existing email. Sets In-Reply-To and References headers correctly. Prefixes subject with 'Re:' if not already present. Quotes the original message body."),
		mcp.WithString("account_id",
			mcp.Description("Account to reply from"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox containing the original email"),
		),
		mcp.WithNumber("uid",
			mcp.Description("UID of the email to reply to"),
			mcp.Required(),
		),
		mcp.WithString("body",
			mcp.Description("Reply body (plain text)"),
			mcp.Required(),
		),
		mcp.WithBoolean("reply_all",
			mcp.Description("Reply to all recipients (default: false)"),
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

		mailbox := req.GetString("mailbox", "INBOX")
		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		body := req.GetString("body", "")
		if body == "" {
			return mcp.NewToolResultError("body is required"), nil
		}
		replyAll := req.GetBool("reply_all", false)

		// Fetch original email to get headers
		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		original, err := imappool.FetchEmail(client, mailbox, uint32(uid), 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("fetch original email failed: %v", err)), nil
		}

		// Build recipients
		to := []string{original.From}
		var cc []string
		if replyAll {
			for _, addr := range original.To {
				if !isSameEmail(addr, acc.Email) {
					cc = append(cc, addr)
				}
			}
			for _, addr := range original.Cc {
				if !isSameEmail(addr, acc.Email) {
					cc = append(cc, addr)
				}
			}
		}

		// Build threading headers (RFC 2822 §3.6.4):
		//   In-Reply-To: <parent-id>
		//   References:  <ref-1> <ref-2> ... <parent-id>
		parentID := ensureAngleBrackets(original.MessageID)
		var refs []string
		for _, id := range original.References {
			refs = append(refs, "<"+id+">")
		}
		if parentID != "" {
			refs = append(refs, parentID)
		}
		references := strings.Join(refs, " ")
		inReplyTo := parentID
		subject := original.Subject

		// Quote original body
		var quotedBody strings.Builder
		quotedBody.WriteString(body)
		quotedBody.WriteString("\n\n")
		quotedBody.WriteString(fmt.Sprintf("On %s, %s wrote:\n", original.Date, original.From))
		for _, line := range strings.Split(original.Body, "\n") {
			quotedBody.WriteString("> " + line + "\n")
		}

		password, err := auth.GetPassword(acc.Auth)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get password: %v", err)), nil
		}

		if err := smtpsender.SendReply(acc, password, to, cc, subject, quotedBody.String(), inReplyTo, references); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("reply failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Reply sent successfully from %s to %s.", acc.Email, original.From)), nil
	})
}

// ensureAngleBrackets normalizes a Message-ID value to the RFC 5322 form
// "<id@host>". IMAP envelopes return Message-IDs with or without brackets
// depending on the server, so we always wrap them before emitting headers.
func ensureAngleBrackets(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if !strings.HasPrefix(id, "<") {
		id = "<" + id
	}
	if !strings.HasSuffix(id, ">") {
		id = id + ">"
	}
	return id
}

// isSameEmail compares a potentially formatted address (e.g. "'Name' <user@host>")
// against a plain email address, ignoring the display name.
func isSameEmail(addr, email string) bool {
	parsed, err := mail.ParseAddress(addr)
	if err == nil {
		return strings.EqualFold(parsed.Address, email)
	}
	// Fallback: try to extract email from angle brackets
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		if j := strings.LastIndex(addr, ">"); j > i {
			return strings.EqualFold(addr[i+1:j], email)
		}
	}
	return strings.EqualFold(addr, email)
}
