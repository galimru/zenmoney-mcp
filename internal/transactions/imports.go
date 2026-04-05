package transactions

import (
	"context"
	"fmt"
	"strings"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func (s *Service) ImportTransactions(ctx context.Context, in ImportTransactionsInput) (ImportTransactionsResponse, error) {
	c, err := s.runtime.Client()
	if err != nil {
		return ImportTransactionsResponse{}, err
	}
	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsWrite)
	if err != nil {
		return ImportTransactionsResponse{}, err
	}

	if len(in.Items) == 0 {
		return ImportTransactionsResponse{}, fmt.Errorf("items array is empty")
	}
	if strings.TrimSpace(in.AccountID) == "" {
		return ImportTransactionsResponse{}, fmt.Errorf("account_id is required")
	}

	plan := &PreparedImportPlan{}
	s.buildImportPlan(ctx, c, env, in.AccountID, in.Items, plan)

	invalidRows := collectInvalidRows(plan)
	if len(invalidRows) > 0 {
		return ImportTransactionsResponse{
			Imported: false,
			Message:  buildBlockedMessage(plan),
			Rows:     invalidRows,
		}, nil
	}

	txs := collectImportableTransactions(plan)

	pushResp, err := c.Push(ctx, pushRequest(s.runtime.CurrentServerTimestamp(), txs))
	if err != nil {
		return ImportTransactionsResponse{
			Imported: false,
			Message:  fmt.Sprintf("Import failed: %v. No transactions were saved. Retry the full batch.", err),
			Rows:     collectInvalidRowsForFailure(plan, err),
		}, nil
	}
	if pushResp.ServerTimestamp > 0 {
		_ = s.runtime.SaveServerTimestamp(pushResp.ServerTimestamp)
	}

	return ImportTransactionsResponse{
		Imported: true,
		Message:  fmt.Sprintf("Imported %d transaction(s) successfully.", len(txs)),
	}, nil
}

func (s *Service) buildImportPlan(ctx context.Context, c client.ZenClient, env *transactionEnv, accountID string, items []ImportDraft, plan *PreparedImportPlan) {
	for i, draft := range items {
		draft.AccountID = accountID
		draft.Currency = ""

		tx, buildErr := buildTransactionFromDraft(ctx, c, env, draft)
		if buildErr != nil {
			plan.Rows = append(plan.Rows, PlannedImportRow{
				Index:      i,
				ExternalID: draft.ExternalID,
				Status:     "invalid",
				Reason:     buildErr.Error(),
			})
			continue
		}

		if status, reason := classifyImportDuplicate(*tx, env, plan); status == "duplicate" {
			plan.Rows = append(plan.Rows, PlannedImportRow{
				Index:      i,
				ExternalID: draft.ExternalID,
				Status:     status,
				Reason:     reason,
			})
		} else {
			plan.Rows = append(plan.Rows, PlannedImportRow{
				Index:      i,
				ExternalID: draft.ExternalID,
				Status:     "new",
				Tx:         tx,
			})
		}
	}
}

func collectInvalidRows(plan *PreparedImportPlan) []ImportInvalidRow {
	var rows []ImportInvalidRow
	for _, row := range plan.Rows {
		if row.Status == "new" {
			continue
		}
		rows = append(rows, ImportInvalidRow{
			Index:      row.Index,
			ExternalID: row.ExternalID,
			Status:     row.Status,
			Reason:     row.Reason,
		})
	}
	return rows
}

func collectImportableTransactions(plan *PreparedImportPlan) []models.Transaction {
	var txs []models.Transaction
	for _, row := range plan.Rows {
		if row.Tx != nil {
			txs = append(txs, *row.Tx)
		}
	}
	return txs
}

func collectInvalidRowsForFailure(plan *PreparedImportPlan, err error) []ImportInvalidRow {
	reason := fmt.Sprintf("push failed: %v", err)
	var rows []ImportInvalidRow
	for _, row := range plan.Rows {
		if row.Tx == nil {
			continue
		}
		rows = append(rows, ImportInvalidRow{
			Index:      row.Index,
			ExternalID: row.ExternalID,
			Status:     "failed",
			Reason:     reason,
		})
	}
	return rows
}

func buildBlockedMessage(plan *PreparedImportPlan) string {
	var lines []string
	for _, row := range plan.Rows {
		if row.Status == "new" {
			continue
		}
		ref := fmt.Sprintf("Row %d", row.Index)
		if row.ExternalID != "" {
			ref += fmt.Sprintf(" (ref: %s)", row.ExternalID)
		}
		label := row.Status
		lines = append(lines, fmt.Sprintf("- %s: %s — %s", ref, label, row.Reason))
	}
	return "Import blocked. Fix the following invalid rows and retry:\n" + strings.Join(lines, "\n")
}

func buildTransactionFromDraft(ctx context.Context, c client.ZenClient, env *transactionEnv, draft ImportDraft) (*models.Transaction, error) {
	tx, err := buildTransaction(ctx, WriteInput{
		Type:        draft.Type,
		Date:        draft.Date,
		Amount:      draft.Amount,
		AccountID:   draft.AccountID,
		ToAccountID: draft.ToAccountID,
		Payee:       draft.Payee,
		PayeeSet:    draft.Payee != "",
		Comment:     draft.Comment,
		CommentSet:  draft.Comment != "",
		Category:    draft.Category,
		Currency:    draft.Currency,
	}, env, c, nil)
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func classifyImportDuplicate(tx models.Transaction, env *transactionEnv, plan *PreparedImportPlan) (string, string) {
	exactKey := duplicateKey(tx)

	for _, planned := range plan.Rows {
		if planned.Tx == nil || classifyTx(*planned.Tx) != classifyTx(tx) {
			continue
		}
		if duplicateKey(*planned.Tx) == exactKey {
			return "duplicate", fmt.Sprintf("matches import row %d", planned.Index)
		}
	}

	for _, existing := range env.resp.Transaction {
		if existing.Deleted || classifyTx(existing) != classifyTx(tx) {
			continue
		}
		if duplicateKey(existing) == exactKey {
			return "duplicate", fmt.Sprintf("matches existing transaction %s", existing.ID)
		}
	}
	return "new", ""
}

func duplicateKey(tx models.Transaction) string {
	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}
	accountID := tx.IncomeAccount
	if tx.OutcomeAccount != nil {
		accountID = *tx.OutcomeAccount
	}
	return strings.Join([]string{
		accountID,
		tx.Date,
		string(classifyTx(tx)),
		fmt.Sprintf("%.2f", majorAmount(tx)),
		normalizeText(tx.Payee),
		normalizeText(comment),
	}, "|")
}
