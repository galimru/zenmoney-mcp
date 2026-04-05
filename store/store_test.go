package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_SaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync_state.json")
	st := New(path)

	state := &SyncState{
		ServerTimestamp: 12345,
		LastSyncAt:      time.Now().UTC().Truncate(time.Second),
	}

	if err := st.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a fresh store to test loading from disk.
	st2 := New(path)
	loaded, err := st2.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil state after load")
	}
	if loaded.ServerTimestamp != 12345 {
		t.Errorf("ServerTimestamp = %d, want 12345", loaded.ServerTimestamp)
	}

	// File should be 0600.
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}

func TestStore_LoadMissingFileReturnsNilNoError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	st := New(path)

	state, err := st.Load()
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for missing file")
	}
}

func TestStore_GetReturnsNilBeforeLoad(t *testing.T) {
	st := New("/tmp/nonexistent-get-test.json")
	state, ok := st.Get()
	if ok || state != nil {
		t.Error("Get should return nil, false before any load/save")
	}
}

func TestStore_Reset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync_state.json")
	st := New(path)

	_ = st.Save(&SyncState{ServerTimestamp: 999})

	if err := st.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// File should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be removed after Reset")
	}

	// Get should return false after reset.
	if _, ok := st.Get(); ok {
		t.Error("Get should return false after Reset")
	}
}

func TestStore_ResetMissingFileIsOk(t *testing.T) {
	st := New("/tmp/no-such-file-reset-test.json")
	if err := st.Reset(); err != nil {
		t.Errorf("Reset on missing file should not error, got: %v", err)
	}
}
