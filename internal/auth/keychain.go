package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	"github.com/zalando/go-keyring"
)

// keychainTimeout bounds a Keychain read. A locked keychain or an access
// prompt nobody sees (server launched by Claude Desktop) would block forever.
const keychainTimeout = 15 * time.Second

func GetPassword(auth config.AuthConfig) (string, error) {
	switch auth.Type {
	case "keychain":
		return getKeychain(auth.KeychainService, auth.KeychainAccount)
	case "env":
		val := os.Getenv(auth.EnvVariable)
		if val == "" {
			return "", fmt.Errorf("environment variable %q is empty", auth.EnvVariable)
		}
		return val, nil
	default:
		return "", fmt.Errorf("unsupported auth type: %q", auth.Type)
	}
}

func getKeychain(service, account string) (string, error) {
	type result struct {
		password string
		err      error
	}
	done := make(chan result, 1)
	go func() {
		p, err := keyring.Get(service, account)
		done <- result{p, err}
	}()
	select {
	case r := <-done:
		return r.password, r.err
	case <-time.After(keychainTimeout):
		return "", fmt.Errorf("keychain read of %q timed out after %s (keychain locked or a macOS access prompt is waiting)", service, keychainTimeout)
	}
}

func SetPassword(service, account, password string) error {
	return keyring.Set(service, account, password)
}
