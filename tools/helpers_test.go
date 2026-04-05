package tools

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/galimru/zenmoney-mcp/store"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func TestBuildLookupMaps(t *testing.T) {
	resp := models.Response{
		Instrument: []models.Instrument{
			{ID: 1, Symbol: "₽", ShortTitle: "RUB"},
			{ID: 2, Symbol: "$", ShortTitle: "USD"},
			{ID: 3, ShortTitle: "EUR"}, // no symbol, falls back to short title
		},
		Account: []models.Account{
			{ID: "acc1", Title: "Cash", Instrument: int32Ptr(1)},
			{ID: "acc2", Title: "Card", Instrument: int32Ptr(2)},
			{ID: "acc3", Title: "No Instrument"},
		},
		Tag: []models.Tag{
			{ID: "tag1", Title: "Food"},
			{ID: "tag2", Title: "Transport"},
		},
	}

	maps := runtime.BuildLookupMaps(resp)

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{"account name", func() string { return maps.AccountName("acc1") }, "Cash"},
		{"account name unknown", func() string { return maps.AccountName("unknown") }, "unknown"},
		{"instrument symbol", func() string { return maps.InstrumentSymbol(1) }, "₽"},
		{"instrument symbol fallback", func() string { return maps.InstrumentSymbol(3) }, "EUR"},
		{"instrument symbol unknown", func() string { return maps.InstrumentSymbol(99) }, "99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	// TagNames
	tagNames := maps.TagNames([]string{"tag1", "tag2", "unknown"})
	if tagNames[0] != "Food" || tagNames[1] != "Transport" || tagNames[2] != "unknown" {
		t.Errorf("unexpected TagNames: %v", tagNames)
	}

	// AccountInstrument
	instrID, ok := maps.AccountInstrument("acc1")
	if !ok || instrID != 1 {
		t.Errorf("expected instrument 1 for acc1, got %d ok=%v", instrID, ok)
	}
	_, ok = maps.AccountInstrument("acc3")
	if ok {
		t.Error("expected no instrument for acc3")
	}
}

func TestLoadSyncState_ResetsOnTokenChange(t *testing.T) {
	st := store.New(filepath.Join(t.TempDir(), "sync_state.json"))
	sessA := runtime.NewSession("token-a", &mockZenClient{}, st, nil)
	_ = sessA.SaveServerTimestamp(123)

	sessB := runtime.NewSession("token-b", &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{ServerTimestamp: 456}, nil
		},
	}, st, nil)
	resp, err := sessB.Incremental(context.Background())
	if err != nil {
		t.Fatalf("Incremental: %v", err)
	}
	if resp.ServerTimestamp != 456 {
		t.Fatalf("ServerTimestamp = %d, want 456", resp.ServerTimestamp)
	}
	if _, ok := st.Get(); ok {
		state, _ := st.Get()
		if state == nil || state.AuthFingerprint != runtime.AuthFingerprint("token-b") {
			t.Fatal("expected state to be replaced for the new token")
		}
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
