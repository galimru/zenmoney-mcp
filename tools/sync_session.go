package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/store"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

var (
	scopeAccounts = []models.EntityType{
		models.EntityTypeAccount,
		models.EntityTypeInstrument,
	}
	scopeTags = []models.EntityType{
		models.EntityTypeTag,
	}
	scopeTagsWithUser = []models.EntityType{
		models.EntityTypeTag,
		models.EntityTypeUser,
	}
	scopeMerchants = []models.EntityType{
		models.EntityTypeMerchant,
	}
	scopeBudgets = []models.EntityType{
		models.EntityTypeBudget,
		models.EntityTypeTag,
	}
	scopeReminders = []models.EntityType{
		models.EntityTypeReminder,
		models.EntityTypeAccount,
		models.EntityTypeTag,
	}
	scopeInstruments = []models.EntityType{
		models.EntityTypeInstrument,
	}
	scopeTransactionsRead = []models.EntityType{
		models.EntityTypeTransaction,
		models.EntityTypeAccount,
		models.EntityTypeTag,
		models.EntityTypeInstrument,
	}
	scopeTransactionsWrite = []models.EntityType{
		models.EntityTypeTransaction,
		models.EntityTypeAccount,
		models.EntityTypeInstrument,
		models.EntityTypeTag,
		models.EntityTypeUser,
	}
	scopeTransactionCreate = []models.EntityType{
		models.EntityTypeAccount,
		models.EntityTypeInstrument,
		models.EntityTypeUser,
		models.EntityTypeTag,
	}
)

type syncSession struct {
	client client.ZenClient
	store  *store.Store
	token  string
}

func newSyncSession(token string, c client.ZenClient, st *store.Store) *syncSession {
	return &syncSession{client: c, store: st, token: token}
}

func (s *syncSession) Incremental(ctx context.Context) (models.Response, error) {
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
		fmt.Printf("warning: failed to save sync state: %v\n", err)
	}
	return resp, nil
}

func (s *syncSession) Full(ctx context.Context) (models.Response, error) {
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

func (s *syncSession) Fetch(ctx context.Context, scope []models.EntityType) (models.Response, error) {
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
			fmt.Printf("warning: failed to save sync state: %v\n", err)
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
		fmt.Printf("warning: failed to save sync state: %v\n", err)
	}
	return resp, nil
}

func (s *syncSession) CurrentServerTimestamp() int {
	state, err := s.loadState()
	if err != nil || state == nil {
		return 0
	}
	return state.ServerTimestamp
}

func (s *syncSession) SaveServerTimestamp(serverTimestamp int) {
	if serverTimestamp <= 0 {
		return
	}
	_ = s.saveState(serverTimestamp)
}

func (s *syncSession) loadState() (*store.SyncState, error) {
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

	want := authFingerprint(s.token)
	if cached.AuthFingerprint != "" && cached.AuthFingerprint != want {
		if err := s.store.Reset(); err != nil {
			return nil, fmt.Errorf("reset sync state: %w", err)
		}
		return nil, nil
	}
	return cached, nil
}

func (s *syncSession) saveState(serverTimestamp int) error {
	return s.store.Save(&store.SyncState{
		ServerTimestamp: serverTimestamp,
		AuthFingerprint: authFingerprint(s.token),
		LastSyncAt:      time.Now(),
	})
}

func authFingerprint(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:8])
}
