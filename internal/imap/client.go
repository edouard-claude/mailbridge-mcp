package imap

import (
	"fmt"
	"sync"

	"github.com/edouard-claude/mailbridge-mcp/internal/auth"
	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Pool maintains one IMAP connection per account.
type Pool struct {
	mu      sync.Mutex
	clients map[string]*imapclient.Client
	cfg     *config.Config
}

func NewPool(cfg *config.Config) *Pool {
	return &Pool{
		clients: make(map[string]*imapclient.Client),
		cfg:     cfg,
	}
}

// Get returns an active IMAP client for the given account.
func (p *Pool) Get(accountID string) (*imapclient.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	acc := p.cfg.Account(accountID)
	if acc == nil {
		return nil, fmt.Errorf("unknown account: %q", accountID)
	}

	if c, ok := p.clients[accountID]; ok {
		if err := c.Noop().Wait(); err == nil {
			return c, nil
		}
		c.Close()
		delete(p.clients, accountID)
	}

	c, err := p.connect(acc)
	if err != nil {
		return nil, err
	}

	p.clients[accountID] = c
	return c, nil
}

func (p *Pool) connect(acc *config.Account) (*imapclient.Client, error) {
	var c *imapclient.Client
	var err error

	if acc.IMAP.TLS {
		c, err = imapclient.DialTLS(acc.IMAP.Addr(), nil)
	} else {
		c, err = imapclient.DialStartTLS(acc.IMAP.Addr(), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("IMAP connect to %s: %w", acc.IMAP.Addr(), err)
	}

	password, err := auth.GetPassword(acc.Auth)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("get password for %s: %w", acc.Email, err)
	}

	if err := c.Login(acc.Email, password).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("IMAP login as %s: %w", acc.Email, err)
	}

	return c, nil
}

// Close closes all IMAP connections.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.clients {
		_ = c.Logout().Wait()
		c.Close()
		delete(p.clients, id)
	}
}
