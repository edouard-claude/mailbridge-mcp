package auth

import (
	"fmt"
	"os"

	"github.com/edouard-claude/mailbridge-mcp/internal/config"
	"github.com/zalando/go-keyring"
)

func GetPassword(auth config.AuthConfig) (string, error) {
	switch auth.Type {
	case "keychain":
		return keyring.Get(auth.KeychainService, auth.KeychainAccount)
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

func SetPassword(service, account, password string) error {
	return keyring.Set(service, account, password)
}
