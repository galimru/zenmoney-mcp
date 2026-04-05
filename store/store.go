package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SyncState is persisted between server sessions to enable incremental sync.
type SyncState struct {
	// ServerTimestamp is the Unix epoch second returned by ZenMoney as serverTimestamp.
	// Passed as serverTimestamp in the next sync request to fetch only newer changes.
	ServerTimestamp int `json:"server_timestamp"`
	// AuthFingerprint identifies the token/account context that produced this state.
	AuthFingerprint string `json:"auth_fingerprint,omitempty"`
	// LastSyncAt is the wall-clock time of the most recent sync (diagnostic only).
	LastSyncAt time.Time `json:"last_sync_at"`
}

// Store manages loading, saving, and in-memory caching of the sync state.
type Store struct {
	mu    sync.Mutex
	path  string
	state *SyncState
}

// New creates a Store that persists to path.
func New(path string) *Store {
	return &Store{path: path}
}

// DefaultPath returns ~/.config/zenmoney-mcp/sync_state.json.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "sync_state.json"
	}
	return filepath.Join(home, ".config", "zenmoney-mcp", "sync_state.json")
}

// Get returns the in-memory cached state. Returns false if not yet loaded or reset.
func (s *Store) Get() (*SyncState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return nil, false
	}
	cp := *s.state
	return &cp, true
}

// Load reads state from disk and caches it in-memory. Returns nil, nil if the file does
// not exist (first run or after Reset) — callers should treat this as "do a full sync".
func (s *Store) Load() (*SyncState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sync state: %w", err)
	}

	var st SyncState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse sync state: %w", err)
	}
	s.state = &st
	cp := st
	return &cp, nil
}

// Save persists state to disk and updates the in-memory cache.
func (s *Store) Save(state *SyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync state: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write sync state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit sync state: %w", err)
	}

	cp := *state
	s.state = &cp
	return nil
}

// Reset removes the persisted state file and clears the in-memory cache, forcing a full
// sync on the next call. Returns nil if the file does not exist.
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = nil
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
