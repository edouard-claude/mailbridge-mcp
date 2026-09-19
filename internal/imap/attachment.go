package imap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/emersion/go-imap/v2/imapclient"
	message "github.com/emersion/go-message"
)

// maxAttachmentBytes caps how large a single attachment may be when saved to
// disk. It is a var so tests can lower it without writing 100 MiB fixtures.
var maxAttachmentBytes int64 = 100 << 20 // 100 MiB

// ExtractAttachment scans a raw RFC 5322 message and returns the decoded
// content of the attachment whose filename matches, along with its MIME
// type. The traversal mirrors ParsedEmail.walk: multipart containers and
// embedded message/rfc822 parts are descended into, up to maxMIMEDepth, and
// filenames are resolved the same way (partFilename), so what read_email
// lists is exactly what can be extracted here. First match wins.
func ExtractAttachment(raw []byte, filename string) (content []byte, mimeType string, err error) {
	if strings.TrimSpace(filename) == "" {
		return nil, "", fmt.Errorf("filename is required")
	}

	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil && !message.IsUnknownCharset(err) {
		return nil, "", fmt.Errorf("parse message: %w", err)
	}
	if entity == nil {
		return nil, "", fmt.Errorf("parse message: empty entity")
	}

	if content, mimeType, ok := findAttachmentPart(entity, filename, 0); ok {
		return content, mimeType, nil
	}
	return nil, "", fmt.Errorf("attachment %q not found in message", filename)
}

// findAttachmentPart walks the MIME tree looking for the attachment leaf
// whose resolved filename matches.
func findAttachmentPart(e *message.Entity, filename string, depth int) ([]byte, string, bool) {
	if e == nil || depth > maxMIMEDepth {
		return nil, "", false
	}

	mediaType, _, err := e.Header.ContentType()
	if err != nil {
		mediaType = "text/plain"
	}
	mediaType = strings.ToLower(mediaType)

	// Container: recurse into each child part.
	if mr := e.MultipartReader(); mr != nil {
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				if message.IsUnknownCharset(err) && part != nil {
					if c, mt, ok := findAttachmentPart(part, filename, depth+1); ok {
						return c, mt, true
					}
					continue
				}
				break
			}
			if c, mt, ok := findAttachmentPart(part, filename, depth+1); ok {
				return c, mt, true
			}
		}
		return nil, "", false
	}

	// Embedded message (forwarded mail): parse it as a full message.
	if mediaType == "message/rfc822" || mediaType == "message/global" {
		inner, err := message.Read(e.Body)
		if err != nil && !message.IsUnknownCharset(err) {
			return nil, "", false
		}
		if inner != nil {
			return findAttachmentPart(inner, filename, depth+1)
		}
		return nil, "", false
	}

	// Leaf part: same detection rule as walk().
	disp, dispParams, _ := e.Header.ContentDisposition()
	isAttachment := strings.EqualFold(disp, "attachment")

	if isAttachment || !strings.HasPrefix(mediaType, "text/") {
		if partFilename(e, dispParams) == filename {
			data, err := io.ReadAll(e.Body)
			if err != nil {
				return nil, "", false
			}
			return data, mediaType, true
		}
	}
	return nil, "", false
}

// validateAttachmentTarget rejects relative destination directories and
// filenames carrying a path, so a hostile attachment name can never escape
// destDir.
func validateAttachmentTarget(filename, destDir string) error {
	if !filepath.IsAbs(destDir) {
		return fmt.Errorf("dest_dir must be an absolute path")
	}
	if filename == "" || strings.ContainsAny(filename, `/\`) || filename == "." || filename == ".." {
		return fmt.Errorf("invalid filename: %q", filename)
	}
	return nil
}

// saveAttachmentFromRaw extracts the attachment named filename from a raw
// message and writes it to destDir/filename. destDir is created if missing
// and an existing file is overwritten (idempotent). Returns the absolute
// path of the written file, its size and its MIME type.
func saveAttachmentFromRaw(raw []byte, filename, destDir string) (string, int64, string, error) {
	if err := validateAttachmentTarget(filename, destDir); err != nil {
		return "", 0, "", err
	}

	content, mimeType, err := ExtractAttachment(raw, filename)
	if err != nil {
		return "", 0, "", err
	}
	if int64(len(content)) > maxAttachmentBytes {
		return "", 0, "", fmt.Errorf("attachment %q is %d bytes, over the %d MiB limit", filename, len(content), maxAttachmentBytes>>20)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", 0, "", fmt.Errorf("create dest dir %s: %w", destDir, err)
	}

	dest := filepath.Join(destDir, filename)
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		return "", 0, "", fmt.Errorf("write %s: %w", dest, err)
	}

	abs, err := filepath.Abs(dest)
	if err != nil {
		abs = dest
	}
	return abs, int64(len(content)), mimeType, nil
}

// SaveAttachment fetches the raw body of an email by UID, extracts the
// attachment named filename and writes it to destDir/filename. It returns
// the absolute path of the written file, its size in bytes and its MIME
// type. destDir must be an absolute path; it is created if missing and the
// destination file is overwritten, making the operation idempotent.
func SaveAttachment(c *imapclient.Client, mailbox string, uid uint32, filename, destDir string) (string, int64, string, error) {
	if err := validateAttachmentTarget(filename, destDir); err != nil {
		return "", 0, "", err
	}

	raw, err := FetchRawBody(c, mailbox, uid)
	if err != nil {
		return "", 0, "", err
	}
	return saveAttachmentFromRaw(raw, filename, destDir)
}
