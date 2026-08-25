package transactions

import "github.com/nemirlev/zenmoney-go-sdk/v2/models"

// TxType represents the type of a transaction.
type TxType string

const (
	TxTypeExpense  TxType = "expense"
	TxTypeIncome   TxType = "income"
	TxTypeTransfer TxType = "transfer"
)

type FindInput struct {
	DateFrom  string
	DateTo    string
	Account   string
	Category  string
	Query     string
	Payee     string
	MinAmount float64
	MaxAmount float64
	Type      TxType
	Sort      string
	Limit     int
	Offset    int
}

type WriteInput struct {
	Type          TxType
	Date          string
	Amount        float64
	AccountID     string
	ToAccountID   string
	Category      string
	Categories    string
	Payee         string
	PayeeSet      bool
	Comment       string
	CommentSet    bool
	Currency      string
	ClearCategory bool
	ClearPayee    bool
	ClearComment  bool
}

type EditInput struct {
	TransactionID string
	WriteInput
}

type SuggestCategoriesInput struct {
	TransactionIDs []string
}

type ImportTransactionsInput struct {
	AccountID string
	Items     []ImportDraft
}

type ImportDraft struct {
	ExternalID  string  `json:"external_id,omitempty"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	Type        TxType  `json:"type,omitempty"`
	AccountID   string  `json:"account_id,omitempty"`
	ToAccountID string  `json:"to_account_id,omitempty"`
	Payee       string  `json:"payee,omitempty"`
	Comment     string  `json:"comment,omitempty"`
	Category    string  `json:"category,omitempty"`
	Currency    string  `json:"currency,omitempty"`
}

type TransactionResult struct {
	ID              string   `json:"id"`
	Date            string   `json:"date"`
	Type            TxType   `json:"type"`
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

type PaginatedTransactions struct {
	Items  []TransactionResult `json:"items"`
	Total  int                 `json:"total"`
	Offset int                 `json:"offset"`
	Limit  int                 `json:"limit"`
}

type SuggestedTransactionCategory struct {
	Transaction         TransactionResult `json:"transaction"`
	CurrentCategories   []string          `json:"current_categories"`
	SuggestedCategories []string          `json:"suggested_categories,omitempty"`
	Status              string            `json:"status"`
	Reason              string            `json:"reason,omitempty"`
}

type SuggestCategoriesResponse struct {
	Ready       int                            `json:"ready"`
	NeedsReview int                            `json:"needs_review"`
	Skipped     int                            `json:"skipped"`
	Items       []SuggestedTransactionCategory `json:"items"`
}

type ImportInvalidRow struct {
	Index      int    `json:"index"`
	ExternalID string `json:"external_id,omitempty"`
	Status     string `json:"status"` // "duplicate", "invalid", or "failed"
	Reason     string `json:"reason"`
}

type ImportTransactionsResponse struct {
	Imported bool               `json:"imported"`
	Message  string             `json:"message"`
	Rows     []ImportInvalidRow `json:"rows,omitempty"`
}

type DeleteResult struct {
	Message     string            `json:"message"`
	Transaction TransactionResult `json:"transaction"`
}

type PlannedImportRow struct {
	Index      int                 `json:"index"`
	ExternalID string              `json:"external_id,omitempty"`
	Status     string              `json:"status"`
	Reason     string              `json:"reason,omitempty"`
	Tx         *models.Transaction `json:"-"`
}

type PreparedImportPlan struct {
	Rows []PlannedImportRow
}

// EditBatchInput describes a batch of edits applied to existing transactions.
type EditBatchInput struct {
	Items     []EditInput
	ChunkSize int
	// ReturnItems overrides whether the saved rows are echoed back. When nil,
	// they are echoed only for small batches: a large batch would otherwise
	// return thousands of lines the caller already knows it sent.
	ReturnItems *bool
}

// EditBatchRow reports a row that kept the batch from being applied as a whole.
type EditBatchRow struct {
	Index         int    `json:"index"`
	TransactionID string `json:"transaction_id,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
}

// EditBatchResponse is the outcome of a batch edit.
type EditBatchResponse struct {
	Updated bool                `json:"updated"`
	Message string              `json:"message"`
	Count   int                 `json:"count,omitempty"`
	Chunks  int                 `json:"chunks,omitempty"`
	Rows    []EditBatchRow      `json:"rows,omitempty"`
	Items   []TransactionResult `json:"items,omitempty"`
	// ItemsOmitted counts saved rows left out of Items to keep the response small.
	ItemsOmitted int `json:"items_omitted,omitempty"`
}

// EditDraft is one row of a batch edit as received from a tool call.
// Payee and Comment are pointers so an omitted field is distinguishable from
// an explicitly empty one.
type EditDraft struct {
	TransactionID string  `json:"transaction_id"`
	Type          TxType  `json:"type,omitempty"`
	Date          string  `json:"date,omitempty"`
	Amount        float64 `json:"amount,omitempty"`
	AccountID     string  `json:"account_id,omitempty"`
	ToAccountID   string  `json:"to_account_id,omitempty"`
	Category      string  `json:"category,omitempty"`
	Categories    string  `json:"categories,omitempty"`
	Payee         *string `json:"payee,omitempty"`
	Comment       *string `json:"comment,omitempty"`
	Currency      string  `json:"currency,omitempty"`
	ClearCategory bool    `json:"clear_category,omitempty"`
	ClearPayee    bool    `json:"clear_payee,omitempty"`
	ClearComment  bool    `json:"clear_comment,omitempty"`
}

// ToEditInput converts a batch row into the input shared with edit_transaction.
func (d EditDraft) ToEditInput() EditInput {
	in := EditInput{
		TransactionID: d.TransactionID,
		WriteInput: WriteInput{
			Type:          d.Type,
			Date:          d.Date,
			Amount:        d.Amount,
			AccountID:     d.AccountID,
			ToAccountID:   d.ToAccountID,
			Category:      d.Category,
			Categories:    d.Categories,
			Currency:      d.Currency,
			ClearCategory: d.ClearCategory,
			ClearPayee:    d.ClearPayee,
			ClearComment:  d.ClearComment,
		},
	}
	if d.Payee != nil {
		in.Payee = *d.Payee
		in.PayeeSet = true
	}
	if d.Comment != nil {
		in.Comment = *d.Comment
		in.CommentSet = true
	}
	return in
}
