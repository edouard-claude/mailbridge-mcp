# mailbridge-mcp — Spécification technique

## MCP Server IMAP/SMTP multi-comptes en Go pour Claude Desktop

**Version** : 1.0.0
**Protocol** : MCP 2025-11-25
**Langage** : Go >= 1.22
**Transport** : stdio

---

## 1. Vue d'ensemble

`mailbridge-mcp` est un serveur MCP (Model Context Protocol) en Go qui expose des outils de lecture et d'envoi d'emails via IMAP/SMTP. Il est conçu pour être lancé par Claude Desktop comme subprocess stdio, permettant à Claude d'interagir avec les boîtes emails de l'utilisateur.

### 1.1 Objectifs

- Lecture multi-comptes via IMAP (INBOX, dossiers, recherche)
- Envoi et réponse via SMTP
- Gestion des credentials via le Keychain macOS
- Conformité stricte avec la spec MCP 2025-11-25
- Transport stdio uniquement (Claude Desktop local)

### 1.2 Stack technique

| Composant | Bibliothèque | Rôle |
|-----------|-------------|------|
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` | Framework MCP officiel Go |
| IMAP | `github.com/emersion/go-imap/v2` + `go-imap/v2/imapclient` | Connexion IMAP, IDLE |
| SMTP | `net/smtp` (stdlib) + `github.com/emersion/go-sasl` | Envoi d'emails |
| MIME | `github.com/emersion/go-message` | Parsing/construction de messages MIME |
| Keychain | `github.com/zalando/go-keyring` | Stockage sécurisé macOS Keychain |
| Config | Fichier JSON | Configuration des comptes |

---

## 2. Conformité MCP

### 2.1 Transport stdio

Le serveur communique exclusivement via stdin/stdout en JSON-RPC 2.0.

**Règles impératives** :
- Chaque message est un objet JSON-RPC 2.0 complet sur une seule ligne
- Les messages sont délimités par `\n` (newline)
- Les messages NE DOIVENT PAS contenir de newlines intégrées (le JSON doit être compact, pas pretty-printed)
- stdout : uniquement des messages MCP valides (JSON-RPC 2.0)
- stderr : logs (informatif, debug, erreurs) ; le client peut les ignorer
- stdin : uniquement des messages MCP valides reçus du client
- Encodage : UTF-8 obligatoire

```go
// Pseudo-code du transport stdio
scanner := bufio.NewScanner(os.Stdin)
for scanner.Scan() {
    line := scanner.Bytes()
    msg, err := parseJSONRPC(line)
    // ... handle message
}
```

### 2.2 Lifecycle

Le serveur DOIT implémenter le lifecycle complet :

#### Phase 1 : Initialization

Le client envoie `initialize` ; le serveur répond avec ses capabilities.

**Request du client** :
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-11-25",
    "capabilities": {},
    "clientInfo": {
      "name": "claude-desktop",
      "version": "3.x"
    }
  }
}
```

**Response du serveur** :
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-11-25",
    "capabilities": {
      "tools": {
        "listChanged": false
      }
    },
    "serverInfo": {
      "name": "mailbridge-mcp",
      "version": "1.0.0",
      "description": "MCP server for multi-account IMAP/SMTP email access"
    },
    "instructions": "Email bridge for reading, searching, and sending emails across multiple accounts. Credentials are stored securely in macOS Keychain."
  }
}
```

#### Phase 2 : Initialized notification

Le client envoie une notification (sans `id`) :
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

Le serveur NE DOIT PAS répondre à cette notification. Après réception, le serveur passe en mode opérationnel.

#### Phase 3 : Operation

Échange normal de messages (tools/list, tools/call, etc.).

#### Phase 4 : Shutdown

- Le client ferme stdin
- Le serveur détecte EOF et termine proprement (fermeture des connexions IMAP)
- Si le serveur ne termine pas dans un délai raisonnable, le client envoie SIGTERM puis SIGKILL

### 2.3 Version negotiation

- Le serveur DOIT supporter `protocolVersion: "2025-11-25"`
- Si le client demande une version non supportée, le serveur répond avec la version qu'il supporte
- Si le client ne supporte pas la version du serveur, il déconnecte

### 2.4 Error codes JSON-RPC

| Code | Signification |
|------|--------------|
| -32700 | Parse error (JSON invalide) |
| -32600 | Invalid request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |

### 2.5 Gestion des erreurs outil

Deux niveaux de signalement :
1. **Protocol errors** : erreurs JSON-RPC standard (method inconnue, params invalides)
2. **Tool execution errors** : retournés dans le résultat avec `isError: true`

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "IMAP connection failed: authentication error for account work@example.com"
      }
    ],
    "isError": true
  }
}
```

---

## 3. Configuration

### 3.1 Fichier de configuration

Emplacement : `~/.config/mailbridge/accounts.json`

```json
{
  "accounts": [
    {
      "id": "perso",
      "label": "Email perso",
      "email": "user@example.com",
      "imap": {
        "host": "imap.example.com",
        "port": 993,
        "tls": true
      },
      "smtp": {
        "host": "smtp.example.com",
        "port": 587,
        "starttls": true
      },
      "auth": {
        "type": "keychain",
        "keychain_service": "mailbridge-mcp",
        "keychain_account": "user@example.com"
      }
    },
    {
      "id": "pro",
      "label": "Email pro OVH",
      "email": "contact@squirrel.fr",
      "imap": {
        "host": "ssl0.ovh.net",
        "port": 993,
        "tls": true
      },
      "smtp": {
        "host": "ssl0.ovh.net",
        "port": 587,
        "starttls": true
      },
      "auth": {
        "type": "keychain",
        "keychain_service": "mailbridge-mcp",
        "keychain_account": "contact@squirrel.fr"
      }
    }
  ],
  "defaults": {
    "max_fetch": 50,
    "body_max_chars": 10000,
    "timeout_seconds": 30
  }
}
```

### 3.2 Types d'authentification

```go
type AuthConfig struct {
    Type            string `json:"type"`             // "keychain" | "env" | "oauth2"
    KeychainService string `json:"keychain_service"` // pour type "keychain"
    KeychainAccount string `json:"keychain_account"` // pour type "keychain"
    EnvVariable     string `json:"env_variable"`     // pour type "env"
}
```

| Type | Description |
|------|------------|
| `keychain` | Lecture du mot de passe via `go-keyring` dans le Keychain macOS |
| `env` | Lecture depuis une variable d'environnement (fallback) |
| `oauth2` | Réservé pour implémentation future (Gmail, Outlook) |

### 3.3 Keychain macOS

Utiliser `github.com/zalando/go-keyring` (cross-platform, pas de CGo requis) :

```go
import "github.com/zalando/go-keyring"

// Lecture
password, err := keyring.Get(account.Auth.KeychainService, account.Auth.KeychainAccount)

// Écriture (CLI setup)
err := keyring.Set("mailbridge-mcp", "user@example.com", "password123")
```

**CLI de setup** : fournir une commande `mailbridge-mcp setup` qui demande interactivement les credentials et les stocke dans le Keychain :

```
$ mailbridge-mcp setup
Account ID: perso
Email: user@example.com
Password: ********
Stored in Keychain as mailbridge-mcp/user@example.com
```

### 3.4 Configuration Claude Desktop

Fichier : `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mailbridge": {
      "command": "/usr/local/bin/mailbridge-mcp",
      "args": ["serve"],
      "env": {}
    }
  }
}
```

Si installé via `go install` :
```json
{
  "mcpServers": {
    "mailbridge": {
      "command": "/Users/<username>/go/bin/mailbridge-mcp",
      "args": ["serve"]
    }
  }
}
```

---

## 4. Outils MCP

### 4.1 Vue d'ensemble

| Outil | Description | Annotations |
|-------|------------|-------------|
| `list_accounts` | Liste les comptes configurés | readOnly |
| `list_mailboxes` | Liste les dossiers d'un compte | readOnly |
| `search_emails` | Recherche d'emails | readOnly |
| `read_email` | Lecture d'un email complet | readOnly |
| `send_email` | Envoi d'un email | destructive |
| `reply_email` | Réponse à un email | destructive |
| `move_email` | Déplacement d'un email | destructive |
| `mark_email` | Marquer lu/non-lu/flag | destructive |

### 4.2 tools/list

Le serveur répond à `tools/list` avec la liste complète :

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

Response :
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "list_accounts",
        "description": "List all configured email accounts with their ID, label, and email address",
        "inputSchema": {
          "type": "object",
          "additionalProperties": false
        },
        "annotations": {
          "readOnlyHint": true,
          "destructiveHint": false,
          "idempotentHint": true,
          "openWorldHint": false
        }
      }
    ]
  }
}
```

### 4.3 Définition détaillée de chaque outil

#### list_accounts

```json
{
  "name": "list_accounts",
  "description": "List all configured email accounts with their ID, label, and email address. Use this first to discover available accounts before other operations.",
  "inputSchema": {
    "type": "object",
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": true,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": false
  }
}
```

**Résultat attendu** :
```json
{
  "content": [
    {
      "type": "text",
      "text": "Configured accounts:\n- perso: Email perso (user@example.com)\n- pro: Email pro OVH (contact@squirrel.fr)"
    }
  ]
}
```

#### list_mailboxes

```json
{
  "name": "list_mailboxes",
  "description": "List all mailbox folders (INBOX, Sent, Drafts, etc.) for a given email account.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account identifier from list_accounts (e.g. 'perso', 'pro')"
      }
    },
    "required": ["account_id"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": true,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": true
  }
}
```

#### search_emails

```json
{
  "name": "search_emails",
  "description": "Search for emails in a specific account and mailbox. Returns a list of matching email summaries (subject, from, date, UID). Supports filtering by subject, sender, date range, and read/unread status.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account identifier"
      },
      "mailbox": {
        "type": "string",
        "description": "Mailbox name (default: INBOX)",
        "default": "INBOX"
      },
      "query": {
        "type": "string",
        "description": "Free-text search query (searched in subject and body via IMAP SEARCH)"
      },
      "from": {
        "type": "string",
        "description": "Filter by sender email or name"
      },
      "since": {
        "type": "string",
        "description": "Emails since this date (ISO 8601: YYYY-MM-DD)"
      },
      "before": {
        "type": "string",
        "description": "Emails before this date (ISO 8601: YYYY-MM-DD)"
      },
      "unseen_only": {
        "type": "boolean",
        "description": "Only return unread emails",
        "default": false
      },
      "limit": {
        "type": "integer",
        "description": "Max number of results (default: 20, max: 50)",
        "default": 20,
        "minimum": 1,
        "maximum": 50
      }
    },
    "required": ["account_id"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": true,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": true
  }
}
```

**Résultat attendu** :
```json
{
  "content": [
    {
      "type": "text",
      "text": "Found 3 emails in perso/INBOX:\n\n1. [UID:1234] 2026-04-12 | From: alice@example.com | Subject: Meeting tomorrow\n2. [UID:1230] 2026-04-11 | From: bob@test.com | Subject: Invoice #456\n3. [UID:1225] 2026-04-10 | From: newsletter@news.com | Subject: Weekly digest"
    }
  ]
}
```

#### read_email

```json
{
  "name": "read_email",
  "description": "Read the full content of an email by its UID. Returns headers (from, to, cc, date, subject) and body (plain text preferred, HTML stripped if no plain text). Lists attachments with filenames and sizes but does not download them.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account identifier"
      },
      "mailbox": {
        "type": "string",
        "description": "Mailbox name (default: INBOX)",
        "default": "INBOX"
      },
      "uid": {
        "type": "integer",
        "description": "Email UID from search_emails results"
      },
      "max_body_chars": {
        "type": "integer",
        "description": "Max characters to return for body (default: 10000)",
        "default": 10000
      }
    },
    "required": ["account_id", "uid"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": true,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": true
  }
}
```

**Résultat attendu** :
```json
{
  "content": [
    {
      "type": "text",
      "text": "From: alice@example.com\nTo: user@example.com\nCc: bob@example.com\nDate: 2026-04-12T10:30:00+04:00\nSubject: Meeting tomorrow\nMessage-ID: <abc123@example.com>\n\n---\n\nHi,\n\nJust confirming our meeting tomorrow at 2pm.\n\nBest,\nAlice\n\n---\nAttachments:\n- agenda.pdf (24 KB)"
    }
  ]
}
```

#### send_email

```json
{
  "name": "send_email",
  "description": "Send a new email from a configured account. Supports plain text body, CC, BCC. Does NOT support attachments (use reply_email for replies).",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account to send from"
      },
      "to": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Recipient email addresses"
      },
      "cc": {
        "type": "array",
        "items": { "type": "string" },
        "description": "CC recipients"
      },
      "bcc": {
        "type": "array",
        "items": { "type": "string" },
        "description": "BCC recipients"
      },
      "subject": {
        "type": "string",
        "description": "Email subject"
      },
      "body": {
        "type": "string",
        "description": "Plain text email body"
      }
    },
    "required": ["account_id", "to", "subject", "body"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": false,
    "destructiveHint": true,
    "idempotentHint": false,
    "openWorldHint": true
  }
}
```

#### reply_email

```json
{
  "name": "reply_email",
  "description": "Reply to an existing email. Sets In-Reply-To and References headers correctly. Prefixes subject with 'Re:' if not already present. Quotes the original message body.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account to reply from"
      },
      "mailbox": {
        "type": "string",
        "description": "Mailbox containing the original email",
        "default": "INBOX"
      },
      "uid": {
        "type": "integer",
        "description": "UID of the email to reply to"
      },
      "body": {
        "type": "string",
        "description": "Reply body (plain text)"
      },
      "reply_all": {
        "type": "boolean",
        "description": "Reply to all recipients (default: false)",
        "default": false
      }
    },
    "required": ["account_id", "uid", "body"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": false,
    "destructiveHint": true,
    "idempotentHint": false,
    "openWorldHint": true
  }
}
```

#### move_email

```json
{
  "name": "move_email",
  "description": "Move an email to a different mailbox folder (e.g. from INBOX to Archive or Trash).",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account identifier"
      },
      "uid": {
        "type": "integer",
        "description": "UID of the email to move"
      },
      "from_mailbox": {
        "type": "string",
        "description": "Source mailbox",
        "default": "INBOX"
      },
      "to_mailbox": {
        "type": "string",
        "description": "Destination mailbox (e.g. 'Archive', 'Trash', 'INBOX.Processed')"
      }
    },
    "required": ["account_id", "uid", "to_mailbox"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": false,
    "destructiveHint": true,
    "idempotentHint": true,
    "openWorldHint": false
  }
}
```

#### mark_email

```json
{
  "name": "mark_email",
  "description": "Change flags on an email: mark as read/unread, flag/unflag, or mark as important.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_id": {
        "type": "string",
        "description": "Account identifier"
      },
      "mailbox": {
        "type": "string",
        "description": "Mailbox containing the email",
        "default": "INBOX"
      },
      "uid": {
        "type": "integer",
        "description": "UID of the email"
      },
      "action": {
        "type": "string",
        "enum": ["read", "unread", "flag", "unflag"],
        "description": "Action to perform"
      }
    },
    "required": ["account_id", "uid", "action"],
    "additionalProperties": false
  },
  "annotations": {
    "readOnlyHint": false,
    "destructiveHint": true,
    "idempotentHint": true,
    "openWorldHint": false
  }
}
```

---

## 5. Architecture Go

### 5.1 Structure du projet

```
mailbridge-mcp/
├── cmd/
│   └── mailbridge-mcp/
│       └── main.go              # Point d'entrée, sous-commandes serve|setup
├── internal/
│   ├── config/
│   │   └── config.go            # Chargement accounts.json
│   ├── auth/
│   │   └── keychain.go          # Abstraction Keychain / env
│   ├── imap/
│   │   ├── client.go            # Connexion IMAP, pool
│   │   ├── search.go            # Recherche IMAP SEARCH
│   │   └── fetch.go             # Fetch messages, parse MIME
│   ├── smtp/
│   │   └── sender.go            # Envoi SMTP, construction MIME
│   └── tools/
│       ├── registry.go          # Enregistrement des outils MCP
│       ├── list_accounts.go
│       ├── list_mailboxes.go
│       ├── search_emails.go
│       ├── read_email.go
│       ├── send_email.go
│       ├── reply_email.go
│       ├── move_email.go
│       └── mark_email.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 5.2 Point d'entrée

```go
package main

import (
    "fmt"
    "os"
)

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
```

### 5.3 Serveur MCP avec le SDK officiel

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/modelcontextprotocol/go-sdk/server"
)

func runServer() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("config: %v", err)
    }

    s := server.NewServer(
        "mailbridge-mcp",
        "1.0.0",
        server.WithInstructions("Email bridge for multi-account IMAP/SMTP access via macOS Keychain."),
    )

    // Register all tools
    tools.RegisterAll(s, cfg)

    // Run over stdio transport
    transport := server.NewStdioTransport()
    if err := s.Run(context.Background(), transport); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

**Note** : si le SDK officiel `go-sdk` n'expose pas encore l'API exacte ci-dessus au moment de l'implémentation, utiliser `github.com/mark3labs/mcp-go` comme alternative. Cette lib implémente la spec 2025-11-25 et fournit un transport stdio prêt à l'emploi. Vérifier l'API de chaque lib et adapter en conséquence.

### 5.4 Enregistrement d'un outil (pattern)

```go
// internal/tools/search_emails.go
package tools

func registerSearchEmails(s *server.Server, cfg *config.Config) {
    s.AddTool(
        mcp.Tool{
            Name:        "search_emails",
            Description: "Search for emails in a specific account and mailbox...",
            InputSchema: mcp.InputSchema{
                Type: "object",
                Properties: map[string]any{
                    "account_id": map[string]any{
                        "type":        "string",
                        "description": "Account identifier",
                    },
                    "mailbox": map[string]any{
                        "type":        "string",
                        "description": "Mailbox name (default: INBOX)",
                        "default":     "INBOX",
                    },
                    // ... other properties
                },
                Required: []string{"account_id"},
            },
        },
        func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            accountID := req.Params.Arguments["account_id"].(string)
            // ... implementation
            return &mcp.CallToolResult{
                Content: []mcp.Content{
                    mcp.TextContent{Type: "text", Text: result},
                },
            }, nil
        },
    )
}
```

### 5.5 Pool de connexions IMAP

```go
// internal/imap/client.go
package imap

import (
    "sync"

    "github.com/emersion/go-imap/v2/imapclient"
)

// Pool maintains one IMAP connection per account.
// Connections are established lazily and reconnected on failure.
type Pool struct {
    mu      sync.Mutex
    clients map[string]*imapclient.Client // key: account_id
    cfg     *config.Config
}

// Get returns an active IMAP client for the given account.
// Creates a new connection if none exists or if the existing one is dead.
func (p *Pool) Get(accountID string) (*imapclient.Client, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    if c, ok := p.clients[accountID]; ok {
        // Check if connection is still alive via NOOP
        if err := c.Noop().Wait(); err == nil {
            return c, nil
        }
        c.Close()
    }

    // Create new connection
    acc := p.cfg.Account(accountID)
    c, err := imapclient.DialTLS(acc.IMAP.Addr(), nil)
    if err != nil {
        return nil, err
    }

    password, err := auth.GetPassword(acc.Auth)
    if err != nil {
        c.Close()
        return nil, err
    }

    if err := c.Login(acc.Email, password).Wait(); err != nil {
        c.Close()
        return nil, err
    }

    p.clients[accountID] = c
    return c, nil
}

// Close ferme toutes les connexions (appelé au shutdown).
func (p *Pool) Close() {
    p.mu.Lock()
    defer p.mu.Unlock()
    for _, c := range p.clients {
        c.Logout()
        c.Close()
    }
}
```

### 5.6 Parsing MIME

```go
// internal/imap/fetch.go
package imap

import (
    "io"
    "strings"

    "github.com/emersion/go-message"
    "github.com/emersion/go-message/mail"
)

// ParsedEmail represents a parsed email message.
type ParsedEmail struct {
    From        string
    To          []string
    Cc          []string
    Date        string
    Subject     string
    MessageID   string
    Body        string      // plain text body
    Attachments []Attachment
}

type Attachment struct {
    Filename string
    Size     int64
    MimeType string
}

// ParseMessage extracts structured data from a raw email.
func ParseMessage(r io.Reader) (*ParsedEmail, error) {
    mr, err := mail.CreateReader(r)
    if err != nil {
        return nil, err
    }

    header := mr.Header
    parsed := &ParsedEmail{
        Subject:   must(header.Subject()),
        MessageID: header.Get("Message-ID"),
        Date:      must(header.Date()).Format(time.RFC3339),
    }

    // Extract addresses
    if from, err := header.AddressList("From"); err == nil && len(from) > 0 {
        parsed.From = from[0].String()
    }
    // ... To, Cc similarly

    // Walk parts
    for {
        part, err := mr.NextPart()
        if err == io.EOF {
            break
        }
        if err != nil {
            return parsed, nil // return what we have
        }

        switch h := part.Header.(type) {
        case *mail.InlineHeader:
            ct, _, _ := h.ContentType()
            if strings.HasPrefix(ct, "text/plain") && parsed.Body == "" {
                body, _ := io.ReadAll(part.Body)
                parsed.Body = string(body)
            }
        case *mail.AttachmentHeader:
            filename, _ := h.Filename()
            parsed.Attachments = append(parsed.Attachments, Attachment{
                Filename: filename,
                MimeType: must2(h.ContentType()),
            })
        }
    }

    return parsed, nil
}
```

### 5.7 Envoi SMTP

```go
// internal/smtp/sender.go
package smtp

import (
    "fmt"
    "net/smtp"
    "strings"
)

// Send sends a plain text email via SMTP.
func Send(acc *config.Account, password string, to []string, cc []string, bcc []string, subject, body string) error {
    addr := fmt.Sprintf("%s:%d", acc.SMTP.Host, acc.SMTP.Port)
    auth := smtp.PlainAuth("", acc.Email, password, acc.SMTP.Host)

    // Build recipients list (to + cc + bcc)
    recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
    recipients = append(recipients, to...)
    recipients = append(recipients, cc...)
    recipients = append(recipients, bcc...)

    // Build message
    var msg strings.Builder
    fmt.Fprintf(&msg, "From: %s\r\n", acc.Email)
    fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(to, ", "))
    if len(cc) > 0 {
        fmt.Fprintf(&msg, "Cc: %s\r\n", strings.Join(cc, ", "))
    }
    fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
    fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
    fmt.Fprintf(&msg, "Content-Type: text/plain; charset=UTF-8\r\n")
    fmt.Fprintf(&msg, "\r\n")
    fmt.Fprintf(&msg, "%s", body)

    return smtp.SendMail(addr, auth, acc.Email, recipients, []byte(msg.String()))
}
```

**Pour les réponses** (`reply_email`) : lire d'abord l'email original via IMAP, extraire `Message-ID`, `From`, `To`, `Cc`, `Subject`, puis construire le message de réponse avec les headers `In-Reply-To` et `References`.

---

## 6. Sécurité

### 6.1 Credentials

- Les mots de passe NE DOIVENT JAMAIS être stockés en clair dans le fichier de config
- Utilisation exclusive du Keychain macOS via `go-keyring`
- Le fichier `accounts.json` contient uniquement les métadonnées de connexion, pas les secrets
- Les credentials en mémoire sont utilisés le temps de la connexion IMAP/SMTP

### 6.2 Validation des entrées

- Valider tous les `account_id` contre la config
- Valider les adresses email (format basique)
- Limiter la taille du body (`max_body_chars`)
- Limiter le nombre de résultats de recherche
- Timeout sur toutes les opérations réseau (30s par défaut)

### 6.3 TLS

- IMAP : TLS obligatoire (port 993) ou STARTTLS
- SMTP : STARTTLS obligatoire (port 587) ou TLS direct (port 465)
- Ne jamais accepter de connexion en clair

---

## 7. Build et installation

### 7.1 Makefile

```makefile
BINARY := mailbridge-mcp
VERSION := 1.0.0

.PHONY: build install clean

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/mailbridge-mcp/

install: build
	cp $(BINARY) /usr/local/bin/

clean:
	rm -f $(BINARY)
```

### 7.2 go.mod

```
module github.com/edouard/mailbridge-mcp

go 1.22

require (
    github.com/modelcontextprotocol/go-sdk v0.x.x
    github.com/emersion/go-imap/v2 v2.x.x
    github.com/emersion/go-message v0.x.x
    github.com/emersion/go-sasl v0.x.x
    github.com/zalando/go-keyring v0.x.x
)
```

### 7.3 Installation

```bash
# Build
make build

# Setup des credentials
./mailbridge-mcp setup

# Test manuel
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./mailbridge-mcp serve

# Installation dans Claude Desktop
make install
# Puis ajouter la config dans claude_desktop_config.json (voir section 3.4)
# Redémarrer Claude Desktop
```

---

## 8. Tests

### 8.1 Tests unitaires

- `internal/config/config_test.go` : chargement et validation de la config
- `internal/auth/keychain_test.go` : mock du keyring pour CI
- `internal/imap/fetch_test.go` : parsing MIME avec des fichiers .eml fixtures
- `internal/tools/*_test.go` : validation des inputSchema et formatage des résultats

### 8.2 Test d'intégration MCP

Script de validation du protocole :

```bash
#!/bin/bash
# test_mcp.sh - Validates MCP lifecycle over stdio

BINARY="./mailbridge-mcp"

# Send initialize + initialized + tools/list
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
  sleep 0.5
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep 0.5
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
  sleep 0.5
} | $BINARY serve 2>/dev/null | jq .
```

**Vérifications attendues** :
- La réponse à `initialize` contient `protocolVersion: "2025-11-25"`
- La réponse à `initialize` contient `capabilities.tools`
- `tools/list` retourne 8 outils
- Chaque outil a un `name`, `description`, `inputSchema` valide
- Les `inputSchema` sont des objets JSON Schema valides

### 8.3 Tests IMAP

Utiliser un serveur IMAP de test local (ex: `greenmail`, `dovecot` en Docker) pour les tests d'intégration complète.

---

## 9. Roadmap

### v1.0.0 (MVP)
- 8 outils de base
- Auth Keychain
- Multi-comptes
- Transport stdio

### v1.1.0
- Support OAuth2 (Gmail, Outlook)
- IMAP IDLE pour notifications temps réel
- Téléchargement de pièces jointes

### v1.2.0
- Envoi de pièces jointes
- Support HTML body
- Cache local des headers pour recherche offline

---

## 10. Références

- [MCP Specification 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
- [MCP Go SDK officiel](https://github.com/modelcontextprotocol/go-sdk)
- [mcp-go (alternative)](https://github.com/mark3labs/mcp-go)
- [go-imap v2](https://github.com/emersion/go-imap)
- [go-message](https://github.com/emersion/go-message)
- [go-keyring](https://github.com/zalando/go-keyring)
- [Claude Desktop MCP Setup](https://modelcontextprotocol.io/docs/develop/connect-local-servers)
- [MCP Transport stdio](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
- [MCP Lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
- [MCP Tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
