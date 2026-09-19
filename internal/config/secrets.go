package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

var secretEnv = map[string]string{
	"typesafe":   "TYPESAFE_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"openai":     "OPENAI_API_KEY",
}

func Providers() []string { return []string{"typesafe", "openrouter", "openai"} }

func Secret(provider string) (string, error) {
	env, ok := secretEnv[provider]
	if !ok {
		return "", fmt.Errorf("unknown credential %q", provider)
	}
	if value := strings.TrimSpace(os.Getenv(env)); value != "" {
		return value, nil
	}
	value, err := keyring.Get(keyringService, provider)
	if err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is not configured; run docket auth set %s", env, provider)
	}
	return value, nil
}

func SetSecret(provider, value string) error {
	if _, ok := secretEnv[provider]; !ok {
		return fmt.Errorf("unknown credential %q", provider)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("credential cannot be empty")
	}
	return keyring.Set(keyringService, provider, strings.TrimSpace(value))
}

func DeleteSecret(provider string) error {
	if _, ok := secretEnv[provider]; !ok {
		return fmt.Errorf("unknown credential %q", provider)
	}
	err := keyring.Delete(keyringService, provider)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

func SecretSource(provider string) string {
	env, ok := secretEnv[provider]
	if !ok {
		return "unknown"
	}
	if strings.TrimSpace(os.Getenv(env)) != "" {
		return env
	}
	if value, err := keyring.Get(keyringService, provider); err == nil && strings.TrimSpace(value) != "" {
		return "keychain"
	}
	return "missing"
}
