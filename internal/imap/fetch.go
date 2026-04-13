package imap

import (
	"fmt"
	"io"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
)

type ParsedEmail struct {
	From        string
	To          []string
	Cc          []string
	Date        string
	Subject     string
	MessageID   string
	Body        string
	Attachments []Attachment
}

type Attachment struct {
	Filename string
	Size     int64
	MimeType string
}

// FetchEmail fetches and parses a single email by UID.
func FetchEmail(c *imapclient.Client, mailbox string, uid uint32, maxBodyChars int) (*ParsedEmail, error) {
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, fmt.Errorf("select %s: %w", mailbox, err)
	}

	uidSet := goimap.UIDSet{}
	uidSet.AddNum(goimap.UID(uid))

	bodySection := &goimap.FetchItemBodySection{}
	fetchOptions := &goimap.FetchOptions{
		Envelope:    true,
		Flags:       true,
		UID:         true,
		BodySection: []*goimap.FetchItemBodySection{bodySection},
	}

	messages, err := c.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("IMAP fetch UID %d: %w", uid, err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("email UID %d not found in %s", uid, mailbox)
	}

	msg := messages[0]
	parsed := &ParsedEmail{}

	// Extract envelope data
	if msg.Envelope != nil {
		parsed.Subject = msg.Envelope.Subject
		parsed.MessageID = msg.Envelope.MessageID
		parsed.Date = msg.Envelope.Date.Format(time.RFC3339)

		if len(msg.Envelope.From) > 0 {
			parsed.From = formatAddress(msg.Envelope.From[0])
		}
		for _, a := range msg.Envelope.To {
			parsed.To = append(parsed.To, formatAddress(a))
		}
		for _, a := range msg.Envelope.Cc {
			parsed.Cc = append(parsed.Cc, formatAddress(a))
		}
	}

	// Parse MIME body
	bodyBytes := msg.FindBodySection(bodySection)
	if bodyBytes != nil {
		parsed.parseBody(bodyBytes, maxBodyChars)
	}

	return parsed, nil
}

func (p *ParsedEmail) parseBody(body []byte, maxChars int) {
	r := strings.NewReader(string(body))
	mr, err := mail.CreateReader(r)
	if err != nil {
		// Fallback: use raw body
		p.Body = truncate(string(body), maxChars)
		return
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			if strings.HasPrefix(ct, "text/plain") && p.Body == "" {
				data, _ := io.ReadAll(part.Body)
				p.Body = truncate(string(data), maxChars)
			}
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			ct, _, _ := h.ContentType()
			p.Attachments = append(p.Attachments, Attachment{
				Filename: filename,
				MimeType: ct,
			})
		}
	}
}

func FormatEmail(parsed *ParsedEmail) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "From: %s\n", parsed.From)
	fmt.Fprintf(&sb, "To: %s\n", strings.Join(parsed.To, ", "))
	if len(parsed.Cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\n", strings.Join(parsed.Cc, ", "))
	}
	fmt.Fprintf(&sb, "Date: %s\n", parsed.Date)
	fmt.Fprintf(&sb, "Subject: %s\n", parsed.Subject)
	fmt.Fprintf(&sb, "Message-ID: %s\n", parsed.MessageID)
	fmt.Fprintf(&sb, "\n---\n\n%s", parsed.Body)

	if len(parsed.Attachments) > 0 {
		fmt.Fprintf(&sb, "\n\n---\nAttachments:\n")
		for _, a := range parsed.Attachments {
			fmt.Fprintf(&sb, "- %s (%s)\n", a.Filename, a.MimeType)
		}
	}
	return sb.String()
}

func formatAddress(a goimap.Address) string {
	email := fmt.Sprintf("%s@%s", a.Mailbox, a.Host)
	if a.Name != "" {
		return fmt.Sprintf("%s <%s>", a.Name, email)
	}
	return email
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}
