package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/edouard-claude/mailbridge-mcp/internal/auth"
	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/edouard-claude/mailbridge-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

var version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: mailbridge-mcp <serve|setup>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServer()
	case "setup":
		runSetup()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runServer() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool := imappool.NewPool(cfg)
	defer pool.Close()

	s := server.NewMCPServer(
		"mailbridge-mcp",
		version,
		server.WithInstructions("Email bridge for reading, searching, and sending emails across multiple accounts. Credentials are stored securely in macOS Keychain."),
	)

	tools.RegisterAll(s, cfg, pool)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSetup() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Account ID (e.g. perso, pro): ")
	accountID, _ := reader.ReadString('\n')
	accountID = strings.TrimSpace(accountID)

	fmt.Print("Email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	fmt.Print("IMAP host (e.g. ssl0.ovh.net, imap.gmail.com): ")
	imapHost, _ := reader.ReadString('\n')
	imapHost = strings.TrimSpace(imapHost)

	smtpDefault := imapHost
	if strings.HasPrefix(imapHost, "imap.") {
		smtpDefault = "smtp." + imapHost[5:]
	}
	fmt.Printf("SMTP host [%s]: ", smtpDefault)
	smtpHost, _ := reader.ReadString('\n')
	smtpHost = strings.TrimSpace(smtpHost)
	if smtpHost == "" {
		smtpHost = smtpDefault
	}

	fmt.Print("Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	service := "mailbridge-mcp"
	if err := auth.SetPassword(service, email, password); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to store in Keychain: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Password stored in Keychain as %s/%s\n", service, email)

	// Load or create config
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "mailbridge")
	configPath := filepath.Join(configDir, "accounts.json")

	cfg := &config.Config{
		Defaults: config.Defaults{
			MaxFetch:       50,
			BodyMaxChars:   10000,
			TimeoutSeconds: 30,
		},
	}

	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// Replace existing account with same ID, or append
	newAcc := config.Account{
		ID:    accountID,
		Label: accountID,
		Email: email,
		IMAP: config.IMAPConfig{
			Host: imapHost,
			Port: 993,
			TLS:  true,
		},
		SMTP: config.SMTPConfig{
			Host:     smtpHost,
			Port:     587,
			StartTLS: true,
		},
		Auth: config.AuthConfig{
			Type:            "keychain",
			KeychainService: service,
			KeychainAccount: email,
		},
	}

	replaced := false
	for i, acc := range cfg.Accounts {
		if acc.ID == accountID {
			cfg.Accounts[i] = newAcc
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Accounts = append(cfg.Accounts, newAcc)
	}

	// Write config
	if err := os.MkdirAll(configDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", configDir, err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", configPath, err)
		os.Exit(1)
	}

	fmt.Printf("Config saved to %s\n", configPath)
	fmt.Printf("\nAccount %q (%s) ready. Run 'mailbridge-mcp serve' to start.\n", accountID, email)
}
