package tools

import (
	"context"
	"fmt"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSaveAttachment(s *server.MCPServer, cfg *config.Config, pool *imappool.Pool) {
	tool := mcp.NewTool("save_attachment",
		mcp.WithDescription("Download an email attachment and write it to a directory on disk, then return the absolute path, size and mime type. Use the filename exactly as listed by read_email (first match wins). dest_dir must be an absolute path and is created if missing; an existing file is overwritten. Filenames containing a path are rejected and attachments over 100 MiB are refused. Read the saved file with local tools afterwards instead of loading megabytes of base64 into the conversation."),
		mcp.WithString("account_id",
			mcp.Description("Account identifier"),
			mcp.Required(),
		),
		mcp.WithString("mailbox",
			mcp.Description("Mailbox name (default: INBOX)"),
		),
		mcp.WithNumber("uid",
			mcp.Description("Email UID from search_emails results"),
			mcp.Required(),
		),
		mcp.WithString("filename",
			mcp.Description("Attachment filename, exactly as listed by read_email"),
			mcp.Required(),
		),
		mcp.WithString("dest_dir",
			mcp.Description("Absolute path of the destination directory (created if missing)"),
			mcp.Required(),
		),
		mcp.WithReadOnlyHintAnnotation(false),
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
		uid := req.GetInt("uid", 0)
		if uid == 0 {
			return mcp.NewToolResultError("uid is required"), nil
		}
		filename := req.GetString("filename", "")
		if filename == "" {
			return mcp.NewToolResultError("filename is required"), nil
		}
		destDir := req.GetString("dest_dir", "")
		if destDir == "" {
			return mcp.NewToolResultError("dest_dir is required"), nil
		}

		client, err := pool.Get(accountID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("IMAP connection failed for %s: %v", accountID, err)), nil
		}

		path, size, mimeType, err := imappool.SaveAttachment(client, mailbox, uint32(uid), filename, destDir)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save attachment failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Saved %s (%s, %d bytes) → %s", filename, mimeType, size, path)), nil
	})
}
