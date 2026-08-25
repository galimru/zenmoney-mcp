package transactions

import (
	"context"
	"fmt"
	"strings"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

const (
	defaultEditBatchChunkSize = 100
	maxEditBatchChunkSize     = 200
	maxEditBatchItems         = 1000
)

type plannedEdit struct {
	index int
	id    string
	tx    models.Transaction
}

// EditBatch applies edits to many existing transactions in one go.
//
// The whole batch is validated before anything is written: if any row is
// unusable, nothing is pushed and every problem row is reported so the caller
// can fix them and retry. Valid batches are pushed in chunks; if a chunk fails
// mid-run the remaining chunks are skipped and the response states how many
// rows were already applied.
func (s *Service) EditBatch(ctx context.Context, in EditBatchInput) (EditBatchResponse, error) {
	if len(in.Items) == 0 {
		return EditBatchResponse{}, fmt.Errorf("items array is empty")
	}
	if len(in.Items) > maxEditBatchItems {
		return EditBatchResponse{}, fmt.Errorf("batch has %d items, the maximum is %d", len(in.Items), maxEditBatchItems)
	}

	chunkSize := in.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultEditBatchChunkSize
	}
	if chunkSize > maxEditBatchChunkSize {
		chunkSize = maxEditBatchChunkSize
	}

	c, err := s.runtime.Client()
	if err != nil {
		return EditBatchResponse{}, err
	}
	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsWrite)
	if err != nil {
		return EditBatchResponse{}, err
	}

	planned, problems := s.planEditBatch(ctx, c, env, in.Items)
	if len(problems) > 0 {
		return EditBatchResponse{
			Updated: false,
			Message: fmt.Sprintf("Nothing was saved: %d of %d row(s) need fixing. Correct them and retry the batch.", len(problems), len(in.Items)),
			Rows:    problems,
		}, nil
	}

	applied := make([]TransactionResult, 0, len(planned))
	chunks := 0
	for start := 0; start < len(planned); start += chunkSize {
		end := min(start+chunkSize, len(planned))
		chunk := planned[start:end]

		txs := make([]models.Transaction, 0, len(chunk))
		for _, row := range chunk {
			txs = append(txs, row.tx)
		}

		pushResp, pushErr := c.Push(ctx, pushRequest(s.runtime.CurrentServerTimestamp(), txs))
		if pushErr != nil {
			return EditBatchResponse{
				Updated: false,
				Message: fmt.Sprintf("Batch stopped after %d of %d row(s) were saved: %v. Retry the rows listed below.", len(applied), len(planned), pushErr),
				Count:   len(applied),
				Chunks:  chunks,
				Rows:    unappliedRows(planned[start:]),
				Items:   applied,
			}, nil
		}
		if pushResp.ServerTimestamp > 0 {
			_ = s.runtime.SaveServerTimestamp(pushResp.ServerTimestamp)
		}

		chunks++
		for _, row := range chunk {
			applied = append(applied, shapeTransaction(row.tx, env.maps))
		}
	}

	return EditBatchResponse{
		Updated: true,
		Message: fmt.Sprintf("Updated %d transaction(s) in %d push(es).", len(applied), chunks),
		Count:   len(applied),
		Chunks:  chunks,
		Items:   applied,
	}, nil
}

func (s *Service) planEditBatch(ctx context.Context, c client.ZenClient, env *transactionEnv, items []EditInput) ([]plannedEdit, []EditBatchRow) {
	planned := make([]plannedEdit, 0, len(items))
	var problems []EditBatchRow
	seen := make(map[string]int, len(items))

	for i, item := range items {
		id := strings.TrimSpace(item.TransactionID)
		if id == "" {
			problems = append(problems, EditBatchRow{
				Index:  i,
				Status: "invalid",
				Reason: "transaction_id is required",
			})
			continue
		}
		if first, ok := seen[id]; ok {
			problems = append(problems, EditBatchRow{
				Index:         i,
				TransactionID: id,
				Status:        "duplicate",
				Reason:        fmt.Sprintf("transaction_id already used by row %d", first),
			})
			continue
		}
		seen[id] = i

		existing, ok := env.txByID[id]
		if !ok {
			problems = append(problems, EditBatchRow{
				Index:         i,
				TransactionID: id,
				Status:        "not_found",
				Reason:        "no transaction with this ID",
			})
			continue
		}

		updated, buildErr := buildTransaction(ctx, item.WriteInput, env, c, &existing)
		if buildErr != nil {
			problems = append(problems, EditBatchRow{
				Index:         i,
				TransactionID: id,
				Status:        "invalid",
				Reason:        buildErr.Error(),
			})
			continue
		}

		planned = append(planned, plannedEdit{index: i, id: id, tx: updated})
	}

	return planned, problems
}

func unappliedRows(rows []plannedEdit) []EditBatchRow {
	out := make([]EditBatchRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, EditBatchRow{
			Index:         row.index,
			TransactionID: row.id,
			Status:        "not_applied",
			Reason:        "skipped after an earlier chunk failed",
		})
	}
	return out
}
