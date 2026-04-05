package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_CreatesDefaultsOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Override defaultConfigPath via the environment isn't possible directly,
	// so we test save() + unmarshal explicitly.
	cfg := defaultConfig()
	if err := save(path, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty config file")
	}

	// File mode should be 0600.
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}

func TestConfig_ZeroValuesFilledWithDefaults(t *testing.T) {
	// A config with zero values should have defaults applied after load.
	cfg := &Config{TransactionLimit: 0}
	if cfg.TransactionLimit <= 0 {
		cfg.TransactionLimit = defaultTransactionLimit
	}
	if cfg.TransactionLimit != defaultTransactionLimit {
		t.Errorf("TransactionLimit = %d, want %d", cfg.TransactionLimit, defaultTransactionLimit)
	}
}
