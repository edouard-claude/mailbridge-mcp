package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

func request(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func TestResolveSignature(t *testing.T) {
	dir := t.TempDir()
	logo := filepath.Join(dir, "logo.png")
	os.WriteFile(logo, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), 0o600)
	svg := filepath.Join(dir, "logo.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0o600)
	htmlFile := filepath.Join(dir, "sig.html")
	os.WriteFile(htmlFile, []byte(`<p>Édouard CLAUDE<br>Squirrel</p><img src="cid:logo">`), 0o600)

	acc := &config.Account{Email: "me@squirrel.fr", Signature: &config.SignatureConfig{
		HTMLFile: htmlFile,
		Images:   []config.SignatureImage{{CID: "logo", Path: logo}, {CID: "vector", Path: svg}},
	}}

	sig, err := resolveSignature(acc, request(nil), "Bonjour")
	if err != nil || sig == nil {
		t.Fatalf("default: %v %v", sig, err)
	}
	if !strings.Contains(sig.Text, "Édouard CLAUDE\nSquirrel") {
		t.Errorf("text not derived from html: %q", sig.Text)
	}
	if len(sig.Images) != 1 || sig.Images[0].ContentType != "image/png" {
		t.Errorf("images: %+v", sig.Images)
	}

	if sig, _ := resolveSignature(acc, request(map[string]any{"signature": false}), "x"); sig != nil {
		t.Errorf("signature=false still returns %+v", sig)
	}

	// Body already signed (draft rewritten from its own content): no duplicate.
	if sig, _ := resolveSignature(acc, request(nil), "Bonjour\n\n-- \nÉdouard   CLAUDE\nSquirrel"); sig != nil {
		t.Errorf("duplicate signature added")
	}

	sig, err = resolveSignature(acc, request(map[string]any{"signature_text": "-- \nEd, en congés"}), "x")
	if err != nil || sig.Text != "Ed, en congés" || sig.HTML != "" {
		t.Errorf("override: %+v %v", sig, err)
	}

	if _, err := resolveSignature(acc, request(map[string]any{"signature_html": `<img src="cid:vector">`}), "x"); err == nil {
		t.Errorf("svg accepted")
	}
	if _, err := resolveSignature(acc, request(map[string]any{"signature_html": `<img src="cid:missing">`}), "x"); err == nil {
		t.Errorf("unknown cid accepted")
	}

	acc.Signature.Disabled = true
	if sig, _ := resolveSignature(acc, request(nil), "x"); sig != nil {
		t.Errorf("disabled signature used")
	}
	if sig, _ := resolveSignature(&config.Account{}, request(nil), "x"); sig != nil {
		t.Errorf("account without signature: %+v", sig)
	}
}
