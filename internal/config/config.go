package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Accounts []Account `json:"accounts"`
	Defaults Defaults  `json:"defaults"`
}

type Account struct {
	ID    string     `json:"id"`
	Label string     `json:"label"`
	Email string     `json:"email"`
	IMAP  IMAPConfig `json:"imap"`
	SMTP  SMTPConfig `json:"smtp"`
	Auth  AuthConfig `json:"auth"`

	Signature *SignatureConfig `json:"signature,omitempty"`
}

// SignatureConfig is the account signature, appended to new messages and
// replies (before the quote). Each version is given inline or as a file; file
// paths are relative to the config directory and read at each use, so edits
// apply without restarting the server. The HTML references images as
// cid:<cid>, e.g. <img src="cid:logo" width="120" height="40" alt="Logo">.
type SignatureConfig struct {
	Disabled bool             `json:"disabled,omitempty"`
	Text     string           `json:"text,omitempty"`
	TextFile string           `json:"text_file,omitempty"`
	HTML     string           `json:"html,omitempty"`
	HTMLFile string           `json:"html_file,omitempty"`
	Images   []SignatureImage `json:"images,omitempty"`
}

// SignatureImage is an image embedded in the message and shown by the HTML
// signature through its Content-ID. ContentType is detected when empty.
type SignatureImage struct {
	CID         string `json:"cid"`
	Path        string `json:"path"`
	ContentType string `json:"content_type,omitempty"`
}

type IMAPConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	TLS  bool   `json:"tls"`
}

func (c IMAPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	StartTLS bool   `json:"starttls"`
	TLS      bool   `json:"tls"`
}

func (c SMTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type AuthConfig struct {
	Type            string `json:"type"`
	KeychainService string `json:"keychain_service"`
	KeychainAccount string `json:"keychain_account"`
	EnvVariable     string `json:"env_variable"`
}

type Defaults struct {
	MaxFetch       int `json:"max_fetch"`
	BodyMaxChars   int `json:"body_max_chars"`
	TimeoutSeconds int `json:"timeout_seconds"`
}

func (c *Config) Account(id string) *Account {
	for i := range c.Accounts {
		if c.Accounts[i].ID == id {
			return &c.Accounts[i]
		}
	}
	return nil
}

// Dir returns the config directory, ~/.config/mailbridge.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "mailbridge"), nil
}

// ResolvePath expands a leading "~/" and makes relative paths relative to the
// config directory.
func ResolvePath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, p), nil
}

func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "accounts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}

	if cfg.Defaults.MaxFetch == 0 {
		cfg.Defaults.MaxFetch = 50
	}
	if cfg.Defaults.BodyMaxChars == 0 {
		cfg.Defaults.BodyMaxChars = 10000
	}
	if cfg.Defaults.TimeoutSeconds == 0 {
		cfg.Defaults.TimeoutSeconds = 30
	}

	if len(cfg.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts configured in %s", path)
	}

	for i, acc := range cfg.Accounts {
		if acc.ID == "" {
			return nil, fmt.Errorf("account %d: missing id", i)
		}
		if acc.Email == "" {
			return nil, fmt.Errorf("account %q: missing email", acc.ID)
		}
		if sig := acc.Signature; sig != nil {
			for _, img := range sig.Images {
				if img.CID == "" || img.Path == "" {
					return nil, fmt.Errorf("account %q: signature image needs cid and path", acc.ID)
				}
			}
		}
	}

	return &cfg, nil
}
