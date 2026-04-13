package smtp

import (
	"crypto/tls"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
)

// extractEmail parses a potentially formatted address like
// "'Name' <user@host>" and returns just "user@host".
func extractEmail(addr string) string {
	parsed, err := mail.ParseAddress(addr)
	if err == nil {
		return parsed.Address
	}
	// Fallback: strip angle brackets manually
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		if j := strings.LastIndex(addr, ">"); j > i {
			return addr[i+1 : j]
		}
	}
	return addr
}

// Send sends a plain text email via SMTP.
func Send(acc *config.Account, password string, to, cc, bcc []string, subject, body string) error {
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)

	msg := buildMessage(acc.Email, to, cc, subject, body, nil)

	if acc.SMTP.TLS {
		return sendTLS(acc, password, recipients, msg)
	}
	return sendStartTLS(acc, password, recipients, msg)
}

// SendReply sends a reply email with proper In-Reply-To and References headers.
func SendReply(acc *config.Account, password string, to, cc []string, subject, body, inReplyTo, references string) error {
	recipients := make([]string, 0, len(to)+len(cc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)

	extraHeaders := map[string]string{
		"In-Reply-To": inReplyTo,
		"References":  references,
	}

	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	msg := buildMessage(acc.Email, to, cc, subject, body, extraHeaders)

	if acc.SMTP.TLS {
		return sendTLS(acc, password, recipients, msg)
	}
	return sendStartTLS(acc, password, recipients, msg)
}

func buildMessage(from string, to, cc []string, subject, body string, extraHeaders map[string]string) []byte {
	var msg strings.Builder
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(to, ", "))
	if len(cc) > 0 {
		fmt.Fprintf(&msg, "Cc: %s\r\n", strings.Join(cc, ", "))
	}
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	for k, v := range extraHeaders {
		if v != "" {
			fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
		}
	}
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "\r\n")
	fmt.Fprintf(&msg, "%s", body)
	return []byte(msg.String())
}

func sendStartTLS(acc *config.Account, password string, recipients []string, msg []byte) error {
	addr := acc.SMTP.Addr()
	host := acc.SMTP.Host

	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP dial %s: %w", addr, err)
	}
	defer c.Close()

	if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return fmt.Errorf("STARTTLS: %w", err)
	}

	authMech := smtp.PlainAuth("", acc.Email, password, host)
	if err := c.Auth(authMech); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}

	if err := c.Mail(acc.Email); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	for _, rcpt := range recipients {
		if err := c.Rcpt(extractEmail(rcpt)); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("SMTP write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close data: %w", err)
	}

	return c.Quit()
}

func sendTLS(acc *config.Account, password string, recipients []string, msg []byte) error {
	addr := acc.SMTP.Addr()
	host := acc.SMTP.Host

	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("TLS dial %s: %w", addr, err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client: %w", err)
	}
	defer c.Close()

	authMech := smtp.PlainAuth("", acc.Email, password, host)
	if err := c.Auth(authMech); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}

	if err := c.Mail(acc.Email); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	for _, rcpt := range recipients {
		if err := c.Rcpt(extractEmail(rcpt)); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("SMTP write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close data: %w", err)
	}

	return c.Quit()
}

