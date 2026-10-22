package tools

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	smtpsender "github.com/edouard-claude/mailbridge-mcp/internal/smtp"
	"github.com/mark3labs/mcp-go/mcp"
)

// Raster formats displayed inline by Gmail, Outlook, Apple Mail and the
// mobile clients; SVG and WebP are not (Gmail, Outlook desktop).
var signatureImageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true}

const maxSignatureImageSize = 1 << 20

// signatureOptions are the per-call parameters shared by send_email,
// reply_email and save_draft.
func signatureOptions() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithBoolean("signature",
			mcp.Description("Append the account signature, placed before the quoted original (default: true). false = no signature. Do not write the signature in body: it is added automatically."),
		),
		mcp.WithString("signature_text",
			mcp.Description("Replace the account signature for this message only (plain text version)."),
		),
		mcp.WithString("signature_html",
			mcp.Description("Replace the account signature for this message only (HTML version, inline styles). May reference the account signature images as cid:<name>. The text version is derived from it when signature_text is empty."),
		),
	}
}

// resolveSignature returns the signature to append to body: the account's,
// the per-call replacement, or nil when disabled, not configured, or already
// present in body (no duplicate when a draft is rewritten).
func resolveSignature(acc *config.Account, req mcp.CallToolRequest, body string) (*smtpsender.Signature, error) {
	if !req.GetBool("signature", true) {
		return nil, nil
	}
	conf := acc.Signature
	if conf == nil {
		conf = &config.SignatureConfig{}
	}

	text, htm := req.GetString("signature_text", ""), req.GetString("signature_html", "")
	if text == "" && htm == "" {
		if conf.Disabled {
			return nil, nil
		}
		var err error
		if text, err = inlineOrFile(conf.Text, conf.TextFile); err != nil {
			return nil, fmt.Errorf("signature text: %w", err)
		}
		if htm, err = inlineOrFile(conf.HTML, conf.HTMLFile); err != nil {
			return nil, fmt.Errorf("signature html: %w", err)
		}
	}
	if strings.TrimSpace(htm) != "" && strings.TrimSpace(text) == "" {
		text = imappool.HTMLToText(htm)
	}
	text = strings.TrimPrefix(strings.TrimLeft(text, "\r\n"), smtpsender.SignatureDelimiter)
	text = strings.TrimPrefix(text, "--\n")
	if strings.TrimSpace(text) == "" && strings.TrimSpace(htm) == "" {
		return nil, nil
	}
	if containsNormalized(body, text) {
		return nil, nil
	}

	sig := &smtpsender.Signature{Text: text, HTML: htm}
	for _, name := range smtpsender.ReferencedCIDs(htm) {
		img, err := loadSignatureImage(conf.Images, name)
		if err != nil {
			return nil, err
		}
		sig.Images = append(sig.Images, img)
	}
	return sig, nil
}

func inlineOrFile(inline, file string) (string, error) {
	if inline != "" || file == "" {
		return inline, nil
	}
	path, err := config.ResolvePath(file)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func loadSignatureImage(images []config.SignatureImage, cid string) (smtpsender.InlineImage, error) {
	for _, img := range images {
		if img.CID != cid {
			continue
		}
		path, err := config.ResolvePath(img.Path)
		if err != nil {
			return smtpsender.InlineImage{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return smtpsender.InlineImage{}, fmt.Errorf("signature image %q: %w", cid, err)
		}
		if len(data) > maxSignatureImageSize {
			return smtpsender.InlineImage{}, fmt.Errorf("signature image %q: %d bytes, max %d", cid, len(data), maxSignatureImageSize)
		}
		ct := img.ContentType
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		if mt, _, err := mime.ParseMediaType(ct); err != nil || !signatureImageTypes[mt] {
			return smtpsender.InlineImage{}, fmt.Errorf("signature image %q: type %q not supported (png, jpeg or gif)", cid, ct)
		}
		return smtpsender.InlineImage{Name: cid, Filename: filepath.Base(path), ContentType: ct, Data: data}, nil
	}
	return smtpsender.InlineImage{}, fmt.Errorf("signature html references cid:%s but the account signature has no image with that cid", cid)
}

// containsNormalized reports whether body already contains sig, ignoring
// whitespace differences.
func containsNormalized(body, sig string) bool {
	sig = strings.Join(strings.Fields(sig), " ")
	return sig != "" && strings.Contains(strings.Join(strings.Fields(body), " "), sig)
}

// describeSignature summarizes the appended signature for tool results.
func describeSignature(sig *smtpsender.Signature) string {
	switch {
	case sig == nil:
		return " No signature."
	case strings.TrimSpace(sig.HTML) == "":
		return " Signature: text."
	default:
		return fmt.Sprintf(" Signature: text + HTML, %d inline image(s).", len(sig.Images))
	}
}
