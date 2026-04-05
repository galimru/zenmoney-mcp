package tools

import (
	"testing"

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

	maps := BuildLookupMaps(resp)

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

func TestClassifyTx(t *testing.T) {
	acc1 := "account-1"
	acc2 := "account-2"

	tests := []struct {
		name string
		tx   models.Transaction
		want string
	}{
		{
			name: "expense",
			tx:   models.Transaction{Income: 0, Outcome: 500, IncomeAccount: acc1, OutcomeAccount: &acc1},
			want: "expense",
		},
		{
			name: "income",
			tx:   models.Transaction{Income: 1000, Outcome: 0, IncomeAccount: acc1, OutcomeAccount: &acc1},
			want: "income",
		},
		{
			name: "transfer",
			tx:   models.Transaction{Income: 1000, Outcome: 1000, IncomeAccount: acc2, OutcomeAccount: &acc1},
			want: "transfer",
		},
		{
			name: "both nonzero same account (income wins)",
			tx:   models.Transaction{Income: 500, Outcome: 500, IncomeAccount: acc1, OutcomeAccount: &acc1},
			want: "income",
		},
		{
			name: "no outcome account pointer",
			tx:   models.Transaction{Income: 0, Outcome: 200, IncomeAccount: acc1, OutcomeAccount: nil},
			want: "expense",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTx(tt.tx); got != tt.want {
				t.Errorf("classifyTx() = %q, want %q", got, tt.want)
			}
		})
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
