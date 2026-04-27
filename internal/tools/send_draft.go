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

func registerSendDraft(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("send_draft",
		mcp.WithDescription("Send a draft email from the Drafts folder. Fetches the draft, sends it via SMTP, then deletes the draft."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithNumber("uid",
			mcp.Description("UID of the draft email to send"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox containing the draft (default: 'Drafts')"),
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

		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		draftsMailbox := req.GetString("mailbox", "Drafts")

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed: %v", err)), nil
		}

		// Fetch the draft
		draft, err := imappool.FetchEmail(client, draftsMailbox, uint32(uid), 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("fetch draft failed: %v", err)), nil
		}

		if len(draft.To) == 0 {
			return mcp.NewToolResultError("draft has no recipients (To field is empty)"), nil
		}

		// Send via SMTP
		password, err := auth.GetPassword(acc.Auth)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get password: %v", err)), nil
		}

		msg, err := smtpsender.Send(acc, password, draft.To, draft.Cc, nil, draft.Subject, draft.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("send failed: %v", err)), nil
		}

		// Copy to Sent folder via IMAP APPEND
		if sentMailbox, err := imappool.FindSentMailbox(client); err == nil {
			imappool.AppendMessage(client, sentMailbox, []goimap.Flag{goimap.FlagSeen}, msg)
		}

		// Delete the draft from Drafts folder
		if err := imappool.DeleteEmail(client, draftsMailbox, uint32(uid)); err != nil {
			return mcp.NewToolResultText(fmt.Sprintf(
				"Email sent to %s, but failed to delete draft: %v",
				strings.Join(draft.To, ", "), err,
			)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Draft sent and removed from %s: \"%s\" → %s",
			draftsMailbox, draft.Subject, strings.Join(draft.To, ", "),
		)), nil
	})
}
