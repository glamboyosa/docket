package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const keyringService = "docket"

type Config struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	LibraryPath string `json:"library_path"`
}

func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("find home directory: %w", err)
	}
	return Config{
		Provider:    "openrouter",
		Model:       "openrouter/auto",
		LibraryPath: filepath.Join(home, "Documents", "Docket"),
	}, nil
}

func ConfigDir() (string, error) {
	if dir := os.Getenv("DOCKET_HOME"); dir != "" {
		return dir, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "docket"), nil
}

func DataDir() (string, error) {
	return ConfigDir()
}

func Load() (Config, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, err
	}
	dir, err := ConfigDir()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Provider == "openrouter" && cfg.Model == "google/gemini-2.5-flash" {
		cfg.Model = "openrouter/auto"
	}
	if cfg.Provider != "openrouter" && cfg.Provider != "openai" {
		return Config{}, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	if cfg.Model == "" || cfg.LibraryPath == "" {
		return Config{}, errors.New("config model and library_path are required")
	}
	return cfg, nil
}

func Save(cfg Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
