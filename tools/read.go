package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// RegisterReadTools adds list_merchants, list_budgets, list_reminders, and list_instruments.
func RegisterReadTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("list_merchants",
			mcp.WithDescription("List all merchants (payees/counterparties)."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListMerchants(ctx, runtime)
		},
	)

	s.AddTool(
		mcp.NewTool("list_budgets",
			mcp.WithDescription("List monthly budgets. Optionally filter by month (format: YYYY-MM)."),
			mcp.WithString("month",
				mcp.Description("Filter by month (YYYY-MM), e.g. 2024-03"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			month := req.GetString("month", "")
			return handleListBudgets(ctx, runtime, month)
		},
	)

	s.AddTool(
		mcp.NewTool("list_reminders",
			mcp.WithDescription("List all recurring transaction reminders."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListReminders(ctx, runtime)
		},
	)

	s.AddTool(
		mcp.NewTool("list_instruments",
			mcp.WithDescription("List all currency instruments with their exchange rates."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListInstruments(ctx, runtime)
		},
	)
}

type merchantResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type budgetResult struct {
	Date    string  `json:"date"`
	Tag     *string `json:"tag,omitempty"` // resolved tag title
	Income  float64 `json:"income"`
	Outcome float64 `json:"outcome"`
}

type reminderResult struct {
	ID             string   `json:"id"`
	Income         float64  `json:"income"`
	IncomeAccount  string   `json:"income_account"`
	Outcome        float64  `json:"outcome"`
	OutcomeAccount string   `json:"outcome_account"`
	Tags           []string `json:"tags"`
	Payee          *string  `json:"payee,omitempty"`
	Comment        string   `json:"comment,omitempty"`
	StartDate      string   `json:"start_date"`
	EndDate        *string  `json:"end_date,omitempty"`
	Interval       *string  `json:"interval,omitempty"`
}

type instrumentResult struct {
	ID         int     `json:"id"`
	Title      string  `json:"title"`
	ShortTitle string  `json:"short_title"`
	Symbol     string  `json:"symbol"`
	Rate       float64 `json:"rate"`
}

func toReminderResult(r models.Reminder, maps LookupMaps) reminderResult {
	tags := r.Tag
	if tags == nil {
		tags = []string{}
	}
	comment := r.Comment
	return reminderResult{
		ID:             r.ID,
		Income:         r.Income,
		IncomeAccount:  maps.AccountName(r.IncomeAccount),
		Outcome:        r.Outcome,
		OutcomeAccount: maps.AccountName(r.OutcomeAccount),
		Tags:           maps.TagNames(tags),
		Payee:          r.Payee,
		Comment:        comment,
		StartDate:      r.StartDate,
		EndDate:        r.EndDate,
		Interval:       r.Interval,
	}
}

func handleListMerchants(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, _, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]merchantResult, len(resp.Merchant))
	for i, m := range resp.Merchant {
		results[i] = merchantResult{ID: m.ID, Title: m.Title}
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleListBudgets(ctx context.Context, runtime *RuntimeProvider, month string) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]budgetResult, 0, len(resp.Budget))
	for _, b := range resp.Budget {
		if month != "" {
			// b.Date is "yyyy-MM-dd"; check prefix match for "yyyy-MM".
			if len(b.Date) < 7 || b.Date[:7] != month {
				continue
			}
		}
		var tagName *string
		if b.Tag != nil && *b.Tag != "" {
			name := maps.Tags[*b.Tag]
			if name == "" {
				name = *b.Tag
			}
			tagName = &name
		}
		results = append(results, budgetResult{
			Date:    b.Date,
			Tag:     tagName,
			Income:  b.Income,
			Outcome: b.Outcome,
		})
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleListReminders(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]reminderResult, len(resp.Reminder))
	for i, r := range resp.Reminder {
		results[i] = toReminderResult(r, maps)
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleListInstruments(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, _, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]instrumentResult, len(resp.Instrument))
	for i, instr := range resp.Instrument {
		results[i] = instrumentResult{
			ID:         instr.ID,
			Title:      instr.Title,
			ShortTitle: instr.ShortTitle,
			Symbol:     instr.Symbol,
			Rate:       instr.Rate,
		}
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}
