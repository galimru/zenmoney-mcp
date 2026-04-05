package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// LookupMaps resolves integer and UUID IDs to human-readable names for tool responses.
type LookupMaps struct {
	Accounts           map[string]string // account UUID → title
	Tags               map[string]string // tag UUID → title
	Instruments        map[int]string    // instrument ID → symbol (e.g. "₽")
	AccountInstruments map[string]int    // account UUID → instrument ID
}

// BuildLookupMaps constructs LookupMaps from a sync response.
func BuildLookupMaps(resp models.Response) LookupMaps {
	m := LookupMaps{
		Accounts:           make(map[string]string, len(resp.Account)),
		Tags:               make(map[string]string, len(resp.Tag)),
		Instruments:        make(map[int]string, len(resp.Instrument)),
		AccountInstruments: make(map[string]int, len(resp.Account)),
	}
	for _, instr := range resp.Instrument {
		sym := instr.Symbol
		if sym == "" {
			sym = instr.ShortTitle
		}
		m.Instruments[instr.ID] = sym
	}
	for _, acc := range resp.Account {
		m.Accounts[acc.ID] = acc.Title
		if acc.Instrument != nil {
			m.AccountInstruments[acc.ID] = int(*acc.Instrument)
		}
	}
	for _, tag := range resp.Tag {
		m.Tags[tag.ID] = tag.Title
	}
	return m
}

// AccountName resolves an account ID to its display title, falling back to the raw ID.
func (m LookupMaps) AccountName(id string) string {
	if name, ok := m.Accounts[id]; ok {
		return name
	}
	return id
}

// TagNames resolves a slice of tag IDs to their titles.
func (m LookupMaps) TagNames(ids []string) []string {
	names := make([]string, len(ids))
	for i, id := range ids {
		if name, ok := m.Tags[id]; ok {
			names[i] = name
		} else {
			names[i] = id
		}
	}
	return names
}

// InstrumentSymbol resolves an instrument ID to its symbol, falling back to the ID as a string.
func (m LookupMaps) InstrumentSymbol(id int) string {
	if sym, ok := m.Instruments[id]; ok {
		return sym
	}
	return fmt.Sprintf("%d", id)
}

// AccountInstrument returns the instrument ID for an account.
func (m LookupMaps) AccountInstrument(accountID string) (int, bool) {
	id, ok := m.AccountInstruments[accountID]
	return id, ok
}

func validateTagIDs(tagIDs []string, maps LookupMaps) error {
	for _, tagID := range tagIDs {
		if _, ok := maps.Tags[tagID]; !ok {
			return fmt.Errorf("tag %q not found", tagID)
		}
	}
	return nil
}

// classifyTx infers the transaction type from its fields.
func classifyTx(tx models.Transaction) string {
	if tx.Income > 0 && tx.Outcome > 0 {
		outcomeAcc := ""
		if tx.OutcomeAccount != nil {
			outcomeAcc = *tx.OutcomeAccount
		}
		if tx.IncomeAccount != outcomeAcc {
			return "transfer"
		}
	}
	if tx.Income > 0 {
		return "income"
	}
	return "expense"
}

// transactionResult is the shaped output for a single transaction.
type transactionResult struct {
	ID              string   `json:"id"`
	Date            string   `json:"date"`
	Type            string   `json:"type"`
	Income          float64  `json:"income"`
	IncomeAccount   string   `json:"income_account"`
	IncomeCurrency  string   `json:"income_currency"`
	Outcome         float64  `json:"outcome"`
	OutcomeAccount  string   `json:"outcome_account"`
	OutcomeCurrency string   `json:"outcome_currency"`
	Tags            []string `json:"tags"`
	Payee           string   `json:"payee,omitempty"`
	Comment         string   `json:"comment,omitempty"`
	Deleted         bool     `json:"deleted,omitempty"`
}

// shapeTransaction maps a models.Transaction to a transactionResult using lookup maps.
func shapeTransaction(tx models.Transaction, maps LookupMaps) transactionResult {
	outcomeAcc := tx.IncomeAccount
	if tx.OutcomeAccount != nil {
		outcomeAcc = *tx.OutcomeAccount
	}

	incomeInstr := maps.InstrumentSymbol(tx.IncomeInstrument)
	outcomeInstr := maps.InstrumentSymbol(tx.OutcomeInstrument)

	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}

	tags := tx.Tag
	if tags == nil {
		tags = []string{}
	}

	return transactionResult{
		ID:              tx.ID,
		Date:            tx.Date,
		Type:            classifyTx(tx),
		Income:          tx.Income,
		IncomeAccount:   maps.AccountName(tx.IncomeAccount),
		IncomeCurrency:  incomeInstr,
		Outcome:         tx.Outcome,
		OutcomeAccount:  maps.AccountName(outcomeAcc),
		OutcomeCurrency: outcomeInstr,
		Tags:            maps.TagNames(tags),
		Payee:           tx.Payee,
		Comment:         comment,
		Deleted:         tx.Deleted,
	}
}

// syncCountResult is the response shape for sync/full_sync tools.
type syncCountResult struct {
	SyncedAt     string `json:"synced_at"`
	Transactions int    `json:"transactions"`
	Accounts     int    `json:"accounts"`
	Tags         int    `json:"tags"`
	Merchants    int    `json:"merchants"`
	Instruments  int    `json:"instruments"`
	Budgets      int    `json:"budgets"`
	Reminders    int    `json:"reminders"`
}

func buildSyncCountResult(resp models.Response) syncCountResult {
	return syncCountResult{
		SyncedAt:     time.Now().UTC().Format(time.RFC3339),
		Transactions: len(resp.Transaction),
		Accounts:     len(resp.Account),
		Tags:         len(resp.Tag),
		Merchants:    len(resp.Merchant),
		Instruments:  len(resp.Instrument),
		Budgets:      len(resp.Budget),
		Reminders:    len(resp.Reminder),
	}
}

// runtimeError wraps a dependency initialization failure as a tool error result.
func runtimeError(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError("zenmoney-mcp initialization failed: " + err.Error())
}

// structJSON marshals v as JSON and returns it as a tool result.
func structJSON(v any) (*mcp.CallToolResult, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(out)), nil
}

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// containsStringFold reports whether slice contains s (case-insensitive).
func containsStringFold(slice []string, s string) bool {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
