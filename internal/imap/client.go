package imap

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/edouard-claude/mailbridge-mcp/internal/auth"
	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	"github.com/emersion/go-imap/v2/imapclient"
)

// noopTimeout bounds the liveness check on a pooled connection. A connection
// left idle across a laptop sleep can be half-open: without this bound NOOP
// waits for TCP keepalive to give up, which takes minutes.
const noopTimeout = 10 * time.Second

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

// Get returns an active IMAP client for the given account. The pool lock is
// not held during network I/O, so Drop can always interrupt a stuck call.
func (p *Pool) Get(accountID string) (*imapclient.Client, error) {
	acc := p.cfg.Account(accountID)
	if acc == nil {
		return nil, fmt.Errorf("unknown account: %q", accountID)
	}

	p.mu.Lock()
	c, ok := p.clients[accountID]
	p.mu.Unlock()

	if ok {
		if err := waitTimeout(c.Noop().Wait, noopTimeout); err == nil {
			return c, nil
		}
		p.remove(accountID, c)
	}

	c, err := p.connect(acc)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if existing, ok := p.clients[accountID]; ok {
		// A concurrent Get connected first: keep a single connection.
		p.mu.Unlock()
		c.Close()
		return existing, nil
	}
	p.clients[accountID] = c
	p.mu.Unlock()
	return c, nil
}

// Drop closes the pooled connection of an account. Pending commands on it
// fail immediately, and the next Get reconnects.
func (p *Pool) Drop(accountID string) {
	p.mu.Lock()
	c, ok := p.clients[accountID]
	delete(p.clients, accountID)
	p.mu.Unlock()
	if ok {
		c.Close()
	}
}

// remove closes c and evicts it, unless the slot already holds another client.
func (p *Pool) remove(accountID string, c *imapclient.Client) {
	p.mu.Lock()
	if p.clients[accountID] == c {
		delete(p.clients, accountID)
	}
	p.mu.Unlock()
	c.Close()
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
		_ = waitTimeout(c.Logout().Wait, noopTimeout)
		c.Close()
		delete(p.clients, id)
	}
}

// waitTimeout runs fn and gives up after d. fn keeps running in the
// background; callers close the connection to unblock it.
func waitTimeout(fn func() error, d time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return errors.New("timed out")
	}
}
