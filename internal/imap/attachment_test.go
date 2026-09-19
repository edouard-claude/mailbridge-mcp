package imap

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// Fixture: multipart/mixed with a base64 PDF attachment and a
// quoted-printable text attachment.
var attachmentPDF = []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\nfake pdf body \x00\x01\x02")

var attachmentMail = "MIME-Version: 1.0\r\n" +
	"Subject: With attachments\r\n" +
	"Content-Type: multipart/mixed; boundary=\"ATT\"\r\n" +
	"\r\n" +
	"--ATT\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"See attached files.\r\n" +
	"--ATT\r\n" +
	"Content-Type: application/pdf\r\n" +
	"Content-Disposition: attachment; filename=\"rapport.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	base64.StdEncoding.EncodeToString(attachmentPDF) + "\r\n" +
	"--ATT\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Disposition: attachment; filename=\"notes.txt\"\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"Caf=C3=A9 chaud, d=C3=A9j=C3=A0 servi.\r\n" +
	"--ATT--\r\n"

// Fixture: inline image with a filename inside multipart/related.
var inlineMail = "MIME-Version: 1.0\r\n" +
	"Subject: Inline image\r\n" +
	"Content-Type: multipart/related; boundary=\"REL\"\r\n" +
	"\r\n" +
	"--REL\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>logo ci-dessous</p>\r\n" +
	"--REL\r\n" +
	"Content-Type: image/png\r\n" +
	"Content-Disposition: inline; filename=\"logo.png\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n")) + "\r\n" +
	"--REL--\r\n"

// Fixture: attachment nested inside a forwarded message/rfc822.
var nestedMail = "MIME-Version: 1.0\r\n" +
	"Subject: Fwd: with zip\r\n" +
	"Content-Type: multipart/mixed; boundary=\"OUT2\"\r\n" +
	"\r\n" +
	"--OUT2\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"wrapper text\r\n" +
	"--OUT2\r\n" +
	"Content-Type: message/rfc822\r\n" +
	"\r\n" +
	"Content-Type: multipart/mixed; boundary=\"IN2\"\r\n" +
	"\r\n" +
	"--IN2\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"inner text\r\n" +
	"--IN2\r\n" +
	"Content-Type: application/zip\r\n" +
	"Content-Disposition: attachment; filename=\"inner.zip\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	base64.StdEncoding.EncodeToString([]byte("PK\x03\x04zip")) + "\r\n" +
	"--IN2--\r\n" +
	"--OUT2--\r\n"

func TestExtractAttachmentBase64(t *testing.T) {
	content, mimeType, err := ExtractAttachment([]byte(attachmentMail), "rapport.pdf")
	if err != nil {
		t.Fatalf("ExtractAttachment(rapport.pdf): %v", err)
	}
	if mimeType != "application/pdf" {
		t.Errorf("mimeType = %q, want application/pdf", mimeType)
	}
	if !bytes.Equal(content, attachmentPDF) {
		t.Errorf("rapport.pdf: got %d bytes, want %d (base64 not decoded?)", len(content), len(attachmentPDF))
	}
}

func TestExtractAttachmentQuotedPrintable(t *testing.T) {
	content, mimeType, err := ExtractAttachment([]byte(attachmentMail), "notes.txt")
	if err != nil {
		t.Fatalf("ExtractAttachment(notes.txt): %v", err)
	}
	if mimeType != "text/plain" {
		t.Errorf("mimeType = %q, want text/plain", mimeType)
	}
	if string(content) != "Café chaud, déjà servi." {
		t.Errorf("notes.txt = %q, want %q", content, "Café chaud, déjà servi.")
	}
}

func TestExtractAttachmentNestedForward(t *testing.T) {
	content, mimeType, err := ExtractAttachment([]byte(nestedMail), "inner.zip")
	if err != nil {
		t.Fatalf("ExtractAttachment(inner.zip): %v", err)
	}
	if mimeType != "application/zip" {
		t.Errorf("mimeType = %q, want application/zip", mimeType)
	}
	if string(content) != "PK\x03\x04zip" {
		t.Errorf("inner.zip = %q", content)
	}
}

func TestExtractAttachmentInlineImage(t *testing.T) {
	p := &ParsedEmail{}
	p.parseBody([]byte(inlineMail), 0, BodyFormatAuto)
	if len(p.Attachments) != 1 || p.Attachments[0].Filename != "logo.png" {
		t.Fatalf("inline image not listed by the read path: %+v", p.Attachments)
	}
	content, _, err := ExtractAttachment([]byte(inlineMail), "logo.png")
	if err != nil {
		t.Fatalf("ExtractAttachment(logo.png): %v", err)
	}
	if string(content) != "\x89PNG\r\n\x1a\n" {
		t.Errorf("logo.png = %q", content)
	}
}

func TestAttachmentSizesListed(t *testing.T) {
	p := &ParsedEmail{}
	p.parseBody([]byte(attachmentMail), 0, BodyFormatAuto)
	if len(p.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2: %+v", len(p.Attachments), p.Attachments)
	}
	sizes := map[string]int64{}
	for _, a := range p.Attachments {
		sizes[a.Filename] = a.Size
	}
	if sizes["rapport.pdf"] != int64(len(attachmentPDF)) {
		t.Errorf("Size(rapport.pdf) = %d, want %d", sizes["rapport.pdf"], len(attachmentPDF))
	}
	if sizes["notes.txt"] != int64(len("Café chaud, déjà servi.")) {
		t.Errorf("Size(notes.txt) = %d, want %d", sizes["notes.txt"], len("Café chaud, déjà servi."))
	}
}

func TestSaveAttachmentFromRawWritesFile(t *testing.T) {
	dest := t.TempDir()
	path, size, mimeType, err := saveAttachmentFromRaw([]byte(attachmentMail), "rapport.pdf", dest)
	if err != nil {
		t.Fatalf("saveAttachmentFromRaw: %v", err)
	}
	if !strings.HasPrefix(path, dest) {
		t.Errorf("path %q escapes dest %q", path, dest)
	}
	if size != int64(len(attachmentPDF)) {
		t.Errorf("size = %d, want %d", size, len(attachmentPDF))
	}
	if mimeType != "application/pdf" {
		t.Errorf("mimeType = %q", mimeType)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !bytes.Equal(got, attachmentPDF) {
		t.Errorf("saved content mismatch: got %d bytes, want %d", len(got), len(attachmentPDF))
	}
	// Idempotent: saving again overwrites without error.
	if _, _, _, err := saveAttachmentFromRaw([]byte(attachmentMail), "rapport.pdf", dest); err != nil {
		t.Errorf("re-save failed: %v", err)
	}
}

func TestSaveAttachmentRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	for _, filename := range []string{"../evil.txt", "sub/evil.txt", "..\\evil.txt", "..", "."} {
		if _, _, _, err := SaveAttachment(nil, "INBOX", 1, filename, dest); err == nil {
			t.Errorf("expected error for filename %q", filename)
		}
	}
	if _, _, _, err := SaveAttachment(nil, "INBOX", 1, "notes.txt", "relative/dir"); err == nil {
		t.Error("expected error for relative dest_dir")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("files written despite validation: %v", entries)
	}
}

func TestSaveAttachmentMissing(t *testing.T) {
	dest := t.TempDir()
	_, _, _, err := saveAttachmentFromRaw([]byte(attachmentMail), "absent.bin", dest)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want not-found error, got %v", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("files written despite error: %v", entries)
	}
}

func TestSaveAttachmentSizeGuard(t *testing.T) {
	old := maxAttachmentBytes
	maxAttachmentBytes = 4
	defer func() { maxAttachmentBytes = old }()

	dest := t.TempDir()
	_, _, _, err := saveAttachmentFromRaw([]byte(attachmentMail), "rapport.pdf", dest)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("want size-limit error, got %v", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("file written despite limit: %v", entries)
	}
}
