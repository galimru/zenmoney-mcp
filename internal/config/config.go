package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const defaultTransactionLimit = 100

// Config holds user-editable settings persisted to disk.
type Config struct {
	// TransactionLimit is the default page size for find_transactions. Default: 100.
	TransactionLimit int `json:"transaction_limit"`
}

func defaultConfig() *Config {
	return &Config{
		TransactionLimit: defaultTransactionLimit,
	}
}

// Load reads the config file, creating it with defaults if it does not exist.
func Load() (*Config, error) {
	path := DefaultConfigPath()
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := save(path, cfg); err != nil {
			return nil, fmt.Errorf("create default config: %w", err)
		}
	} else if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	} else {
		if err := json.Unmarshal(data, cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	if cfg.TransactionLimit <= 0 {
		cfg.TransactionLimit = defaultTransactionLimit
	}
	return cfg, nil
}

func save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// DefaultConfigPath returns ~/.config/zenmoney-mcp/config.json.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(home, ".config", "zenmoney-mcp", "config.json")
}
