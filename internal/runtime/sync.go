package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"time"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/store"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

var (
	ScopeAccounts = []models.EntityType{
		models.EntityTypeAccount,
		models.EntityTypeInstrument,
	}
	ScopeTags = []models.EntityType{
		models.EntityTypeTag,
	}
	ScopeTagsWithUser = []models.EntityType{
		models.EntityTypeTag,
		models.EntityTypeUser,
	}
	ScopeTransactionsRead = []models.EntityType{
		models.EntityTypeTransaction,
		models.EntityTypeAccount,
		models.EntityTypeTag,
		models.EntityTypeInstrument,
	}
	ScopeTransactionsWrite = []models.EntityType{
		models.EntityTypeTransaction,
		models.EntityTypeAccount,
		models.EntityTypeInstrument,
		models.EntityTypeTag,
		models.EntityTypeUser,
	}
)

type Session struct {
	client client.ZenClient
	store  *store.Store
	token  string
	logger *slog.Logger
}

func NewSession(token string, c client.ZenClient, st *store.Store, logger *slog.Logger) *Session {
	return &Session{client: c, store: st, token: token, logger: logger}
}

func (s *Session) Incremental(ctx context.Context) (models.Response, error) {
	state, err := s.loadState()
	if err != nil {
		return models.Response{}, err
	}

	var resp models.Response
	if state != nil && state.ServerTimestamp > 0 {
		resp, err = s.client.SyncSince(ctx, time.Unix(int64(state.ServerTimestamp), 0))
	} else {
		resp, err = s.client.FullSync(ctx)
	}
	if err != nil {
		return models.Response{}, fmt.Errorf("sync: %w", err)
	}

	if err := s.saveState(resp.ServerTimestamp); err != nil {
		s.warn("failed to save sync state", err)
	}
	return resp, nil
}

func (s *Session) Full(ctx context.Context) (models.Response, error) {
	if err := s.store.Reset(); err != nil {
		return models.Response{}, fmt.Errorf("reset sync state: %w", err)
	}

	resp, err := s.client.FullSync(ctx)
	if err != nil {
		return models.Response{}, fmt.Errorf("full sync failed: %w", err)
	}
	if err := s.saveState(resp.ServerTimestamp); err != nil {
		return models.Response{}, fmt.Errorf("save sync state: %w", err)
	}
	return resp, nil
}

func (s *Session) Fetch(ctx context.Context, scope []models.EntityType) (models.Response, error) {
	state, err := s.loadState()
	if err != nil {
		return models.Response{}, err
	}

	if state == nil || state.ServerTimestamp <= 0 {
		resp, err := s.client.FullSync(ctx)
		if err != nil {
			return models.Response{}, fmt.Errorf("sync: %w", err)
		}
		if err := s.saveState(resp.ServerTimestamp); err != nil {
			s.warn("failed to save sync state", err)
		}
		return resp, nil
	}

	resp, err := s.client.Sync(ctx, models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        state.ServerTimestamp,
		ForceFetch:             append([]models.EntityType(nil), scope...),
	})
	if err != nil {
		return models.Response{}, fmt.Errorf("sync: %w", err)
	}
	if err := s.saveState(resp.ServerTimestamp); err != nil {
		s.warn("failed to save sync state", err)
	}
	return resp, nil
}

func (s *Session) CurrentServerTimestamp() int {
	state, err := s.loadState()
	if err != nil || state == nil {
		return 0
	}
	return state.ServerTimestamp
}

func (s *Session) SaveServerTimestamp(serverTimestamp int) error {
	if serverTimestamp <= 0 {
		return nil
	}
	return s.saveState(serverTimestamp)
}

func AuthFingerprint(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:8])
}

func (s *Session) loadState() (*store.SyncState, error) {
	cached, ok := s.store.Get()
	if !ok {
		loaded, err := s.store.Load()
		if err != nil {
			return nil, fmt.Errorf("load sync state: %w", err)
		}
		cached = loaded
	}
	if cached == nil {
		return nil, nil
	}

	want := AuthFingerprint(s.token)
	if cached.AuthFingerprint != "" && cached.AuthFingerprint != want {
		if err := s.store.Reset(); err != nil {
			return nil, fmt.Errorf("reset sync state: %w", err)
		}
		return nil, nil
	}
	return cached, nil
}

func (s *Session) saveState(serverTimestamp int) error {
	return s.store.Save(&store.SyncState{
		ServerTimestamp: serverTimestamp,
		AuthFingerprint: AuthFingerprint(s.token),
		LastSyncAt:      time.Now(),
	})
}

func (s *Session) warn(msg string, err error) {
	if s.logger != nil {
		s.logger.Warn(msg, "error", err)
	}
}
