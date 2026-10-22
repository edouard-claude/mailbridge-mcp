package smtp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

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

// generateMessageID creates a globally unique Message-ID per RFC 5322 §3.6.4.
func generateMessageID(from string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("<%s.%s@%s>", hex.EncodeToString(b), strconv.FormatInt(time.Now().UnixNano(), 36), addrDomain(from))
}

// addrDomain returns the domain of an address, "localhost" when it has none.
func addrDomain(from string) string {
	addr := extractEmail(from)
	if i := strings.LastIndexByte(addr, '@'); i >= 0 && i < len(addr)-1 {
		return addr[i+1:]
	}
	return "localhost"
}

// isASCII reports whether s contains only ASCII characters.
func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// sanitizeHeaderValue strips CR and LF to prevent header injection (RFC 5322 §2.2).
func sanitizeHeaderValue(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// foldHeader wraps a long header value at whitespace boundaries to keep lines
// ≤ 78 characters per RFC 5322 §2.2.3. prefixLen is the length of
// "Header-Name: " already on the first line.
func foldHeader(value string, prefixLen int) string {
	const maxLen = 78
	if prefixLen+len(value) <= maxLen {
		return value
	}
	var buf strings.Builder
	lineLen := prefixLen
	for i, token := range strings.Fields(value) {
		if i > 0 {
			if lineLen+1+len(token) > maxLen {
				buf.WriteString("\r\n ")
				lineLen = 1
			} else {
				buf.WriteByte(' ')
				lineLen++
			}
		}
		buf.WriteString(token)
		lineLen += len(token)
	}
	return buf.String()
}

// SMTP timeouts: net/smtp sets none, so a silent server would block forever.
const (
	dialTimeout    = 30 * time.Second
	sessionTimeout = 2 * time.Minute
)

// Attachment represents a file to attach to an email.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Send sends an email via SMTP. Returns the built message for IMAP Sent copy.
func Send(acc *config.Account, password string, to, cc, bcc []string, subject string, body Body, attachments []Attachment) ([]byte, error) {
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)

	msg := BuildMessage(acc.Email, to, cc, subject, body, attachments, nil)

	var err error
	if acc.SMTP.TLS {
		err = sendTLS(acc, password, recipients, msg)
	} else {
		err = sendStartTLS(acc, password, recipients, msg)
	}
	return msg, err
}

// BuildReply builds a reply message: threading headers set and subject
// prefixed with "Re:" when missing.
func BuildReply(from string, to, cc []string, subject string, body Body, inReplyTo, references string, attachments []Attachment) []byte {
	extraHeaders := map[string]string{
		"In-Reply-To": inReplyTo,
		"References":  references,
	}

	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		subject = "Re: " + subject
	}

	return BuildMessage(from, to, cc, subject, body, attachments, extraHeaders)
}

// BuildMessage constructs an RFC 5322 compliant MIME message (structure
// described on writeBody).
func BuildMessage(from string, to, cc []string, subject string, body Body, attachments []Attachment, extraHeaders map[string]string) []byte {
	var msg strings.Builder

	// Date (RFC 5322 §3.6 — required)
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))

	// From
	fmt.Fprintf(&msg, "From: %s\r\n", sanitizeHeaderValue(from))

	// To
	fmt.Fprintf(&msg, "To: %s\r\n", sanitizeHeaderValue(strings.Join(to, ", ")))

	// Cc
	if len(cc) > 0 {
		fmt.Fprintf(&msg, "Cc: %s\r\n", sanitizeHeaderValue(strings.Join(cc, ", ")))
	}

	// Message-ID (RFC 5322 §3.6.4 — required)
	fmt.Fprintf(&msg, "Message-ID: %s\r\n", generateMessageID(from))

	// Subject (RFC 2047 encoding for non-ASCII)
	subject = sanitizeHeaderValue(subject)
	if isASCII(subject) {
		fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	} else {
		fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	}

	// Extra headers with folding for long values (e.g. References)
	for k, v := range extraHeaders {
		if v != "" {
			v = sanitizeHeaderValue(v)
			fmt.Fprintf(&msg, "%s: %s\r\n", k, foldHeader(v, len(k)+2))
		}
	}

	// MIME headers
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")

	writeBody(&msg, body, from, attachments)

	return []byte(msg.String())
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// SendRaw sends a pre-built RFC 5322 message via SMTP.
// Returns the message unchanged for IMAP Sent copy.
func SendRaw(acc *config.Account, password string, recipients []string, msg []byte) ([]byte, error) {
	var err error
	if acc.SMTP.TLS {
		err = sendTLS(acc, password, recipients, msg)
	} else {
		err = sendStartTLS(acc, password, recipients, msg)
	}
	return msg, err
}

func sendStartTLS(acc *config.Account, password string, recipients []string, msg []byte) error {
	addr := acc.SMTP.Addr()
	host := acc.SMTP.Host

	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("SMTP dial %s: %w", addr, err)
	}
	conn.SetDeadline(time.Now().Add(sessionTimeout))

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP client: %w", err)
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

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("TLS dial %s: %w", addr, err)
	}
	conn.SetDeadline(time.Now().Add(sessionTimeout))

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
