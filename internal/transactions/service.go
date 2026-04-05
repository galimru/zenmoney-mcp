package transactions

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/google/uuid"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type Runtime interface {
	Config() (*config.Config, error)
	Client() (client.ZenClient, error)
	ScopedSync(ctx context.Context, scope []models.EntityType) (models.Response, runtime.LookupMaps, error)
	CurrentServerTimestamp() int
	SaveServerTimestamp(serverTimestamp int) error
}

type Service struct {
	runtime Runtime
}

func NewService(runtime Runtime) *Service {
	return &Service{runtime: runtime}
}

func normalizeFindInput(in *FindInput, cfg *config.Config) {
	if in.Limit <= 0 {
		in.Limit = cfg.TransactionLimit
	}
	if in.Limit > 500 {
		in.Limit = 500
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	if in.Sort == "" {
		in.Sort = "desc"
	}
}

func (s *Service) Find(ctx context.Context, in FindInput) (PaginatedTransactions, error) {
	cfg, err := s.runtime.Config()
	if err != nil {
		return PaginatedTransactions{}, err
	}

	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsRead)
	if err != nil {
		return PaginatedTransactions{}, err
	}

	if in.Account != "" {
		in.Account, err = resolveReadableAccountRef(in.Account, env.maps)
		if err != nil {
			return PaginatedTransactions{}, err
		}
	}
	if in.Category != "" {
		in.Category, err = resolveCategoryRef(in.Category, env.maps)
		if err != nil {
			return PaginatedTransactions{}, err
		}
	}
	normalizeFindInput(&in, cfg)

	filtered := filterTransactions(env.resp.Transaction, selectionFilter{
		AccountID:  in.Account,
		CategoryID: in.Category,
		DateFrom:   in.DateFrom,
		DateTo:     in.DateTo,
		Query:      strings.ToLower(in.Query),
		Payee:      strings.ToLower(in.Payee),
		Type:       in.Type,
		MinAmount:  in.MinAmount,
		MaxAmount:  in.MaxAmount,
	})

	sort.Slice(filtered, func(i, j int) bool {
		if in.Sort == "asc" {
			return filtered[i].Date < filtered[j].Date
		}
		return filtered[i].Date > filtered[j].Date
	})

	total := len(filtered)
	if in.Offset >= len(filtered) {
		filtered = nil
	} else {
		filtered = filtered[in.Offset:]
		if len(filtered) > in.Limit {
			filtered = filtered[:in.Limit]
		}
	}

	items := make([]TransactionResult, len(filtered))
	for i, tx := range filtered {
		items[i] = shapeTransaction(tx, env.maps)
	}

	return PaginatedTransactions{
		Items:  items,
		Total:  total,
		Offset: in.Offset,
		Limit:  in.Limit,
	}, nil
}

func (s *Service) ListUncategorized(ctx context.Context, in FindInput) (PaginatedTransactions, error) {
	cfg, err := s.runtime.Config()
	if err != nil {
		return PaginatedTransactions{}, err
	}

	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsRead)
	if err != nil {
		return PaginatedTransactions{}, err
	}

	normalizeFindInput(&in, cfg)

	filtered := filterTransactions(env.resp.Transaction, selectionFilter{
		Uncategorized: true,
	})

	sort.Slice(filtered, func(i, j int) bool {
		if in.Sort == "asc" {
			return filtered[i].Date < filtered[j].Date
		}
		return filtered[i].Date > filtered[j].Date
	})

	total := len(filtered)
	if in.Offset >= len(filtered) {
		filtered = nil
	} else {
		filtered = filtered[in.Offset:]
		if len(filtered) > in.Limit {
			filtered = filtered[:in.Limit]
		}
	}

	items := make([]TransactionResult, len(filtered))
	for i, tx := range filtered {
		items[i] = shapeTransaction(tx, env.maps)
	}

	return PaginatedTransactions{
		Items:  items,
		Total:  total,
		Offset: in.Offset,
		Limit:  in.Limit,
	}, nil
}

func (s *Service) Add(ctx context.Context, in WriteInput) (TransactionResult, error) {
	c, err := s.runtime.Client()
	if err != nil {
		return TransactionResult{}, err
	}
	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsWrite)
	if err != nil {
		return TransactionResult{}, err
	}

	tx, err := buildTransaction(ctx, in, env, c, nil)
	if err != nil {
		return TransactionResult{}, err
	}

	pushResp, err := c.Push(ctx, pushRequest(s.runtime.CurrentServerTimestamp(), []models.Transaction{tx}))
	if err != nil {
		return TransactionResult{}, fmt.Errorf("create transaction: %w", err)
	}
	if pushResp.ServerTimestamp > 0 {
		_ = s.runtime.SaveServerTimestamp(pushResp.ServerTimestamp)
	}
	return shapeTransaction(tx, env.maps), nil
}

func (s *Service) Edit(ctx context.Context, in EditInput) (*TransactionResult, error) {
	c, err := s.runtime.Client()
	if err != nil {
		return nil, err
	}
	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsWrite)
	if err != nil {
		return nil, err
	}

	existing, ok := env.txByID[in.TransactionID]
	if !ok {
		return nil, nil
	}

	updated, err := buildTransaction(ctx, in.WriteInput, env, c, &existing)
	if err != nil {
		return nil, err
	}

	pushResp, err := c.Push(ctx, pushRequest(s.runtime.CurrentServerTimestamp(), []models.Transaction{updated}))
	if err != nil {
		return nil, fmt.Errorf("update transaction: %w", err)
	}
	if pushResp.ServerTimestamp > 0 {
		_ = s.runtime.SaveServerTimestamp(pushResp.ServerTimestamp)
	}

	out := shapeTransaction(updated, env.maps)
	return &out, nil
}

func (s *Service) Delete(ctx context.Context, transactionID string) (*DeleteResult, error) {
	if strings.TrimSpace(transactionID) == "" {
		return nil, fmt.Errorf("transaction_id is required")
	}

	c, err := s.runtime.Client()
	if err != nil {
		return nil, err
	}
	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsWrite)
	if err != nil {
		return nil, err
	}

	existing, ok := env.txByID[transactionID]
	if !ok {
		return nil, nil
	}

	deletionTx := models.Transaction{
		ID:                transactionID,
		User:              env.userID,
		Changed:           int(time.Now().Unix()),
		Deleted:           true,
		IncomeAccount:     existing.IncomeAccount,
		OutcomeAccount:    existing.OutcomeAccount,
		IncomeInstrument:  existing.IncomeInstrument,
		OutcomeInstrument: existing.OutcomeInstrument,
		Date:              existing.Date,
	}

	pushResp, err := c.Push(ctx, pushRequest(s.runtime.CurrentServerTimestamp(), []models.Transaction{deletionTx}))
	if err != nil {
		return nil, fmt.Errorf("delete transaction: %w", err)
	}
	if pushResp.ServerTimestamp > 0 {
		_ = s.runtime.SaveServerTimestamp(pushResp.ServerTimestamp)
	}

	return &DeleteResult{
		Message:     fmt.Sprintf("Transaction %s deleted", transactionID),
		Transaction: shapeTransaction(existing, env.maps),
	}, nil
}

func (s *Service) SuggestCategories(ctx context.Context, in SuggestCategoriesInput) (SuggestCategoriesResponse, error) {
	c, err := s.runtime.Client()
	if err != nil {
		return SuggestCategoriesResponse{}, err
	}
	env, err := s.loadEnv(ctx, runtime.ScopeTransactionsWrite)
	if err != nil {
		return SuggestCategoriesResponse{}, err
	}

	selected, err := selectTransactionsByID(in.TransactionIDs, env)
	if err != nil {
		return SuggestCategoriesResponse{}, err
	}

	items := make([]SuggestedTransactionCategory, 0, len(selected))
	ready := 0
	needsReview := 0
	skipped := 0

	for _, tx := range selected {
		item := SuggestedTransactionCategory{
			Transaction:       shapeTransaction(tx, env.maps),
			CurrentCategories: env.maps.TagNames(tx.Tag),
		}

		nextTags, source, reviewReason, err := resolveCategoriesForTransaction(ctx, c, env, tx, true)
		if err != nil {
			return SuggestCategoriesResponse{}, err
		}

		item.SuggestedCategories = env.maps.TagNames(nextTags)
		switch {
		case reviewReason != "":
			item.Status = "needs_review"
			item.Reason = reviewReason
			needsReview++
		case len(nextTags) == 0:
			item.Status = "skipped"
			item.Reason = "no clear category"
			skipped++
		case equalStringSlices(tx.Tag, nextTags):
			item.Status = "skipped"
			item.Reason = "already categorized"
			skipped++
		default:
			item.Status = "ready"
			item.Reason = source
			ready++
		}

		items = append(items, item)
	}

	return SuggestCategoriesResponse{
		Ready:       ready,
		NeedsReview: needsReview,
		Skipped:     skipped,
		Items:       items,
	}, nil
}

type transactionEnv struct {
	resp   models.Response
	maps   runtime.LookupMaps
	userID int
	txByID map[string]models.Transaction
}


func (s *Service) loadEnv(ctx context.Context, scope []models.EntityType) (*transactionEnv, error) {
	resp, maps, err := s.runtime.ScopedSync(ctx, scope)
	if err != nil {
		return nil, err
	}

	env := &transactionEnv{
		resp:   resp,
		maps:   maps,
		txByID: make(map[string]models.Transaction, len(resp.Transaction)),
	}
	for _, tx := range resp.Transaction {
		env.txByID[tx.ID] = tx
	}
	if len(resp.User) > 0 {
		env.userID = resp.User[0].ID
	}
	return env, nil
}

func buildTransaction(ctx context.Context, in WriteInput, env *transactionEnv, c client.ZenClient, existing *models.Transaction) (models.Transaction, error) {
	txType := in.Type
	if txType == "" && existing == nil {
		return models.Transaction{}, fmt.Errorf("type is required")
	}
	if in.Date == "" && existing == nil {
		return models.Transaction{}, fmt.Errorf("date is required (YYYY-MM-DD)")
	}
	if in.Amount <= 0 && existing == nil {
		return models.Transaction{}, fmt.Errorf("amount must be a positive number")
	}
	if in.AccountID == "" && existing == nil {
		return models.Transaction{}, fmt.Errorf("account_id is required")
	}

	requestedTagIDs, requestedExplicit, err := resolveRequestedTags(in.Category, in.Categories, env)
	if err != nil {
		return models.Transaction{}, err
	}

	var tx models.Transaction
	if existing != nil {
		tx = *existing
	} else {
		now := int(time.Now().Unix())
		tx = models.Transaction{
			ID:      uuid.New().String(),
			User:    env.userID,
			Changed: now,
			Created: now,
		}
	}

	if in.Date != "" {
		if _, err := time.Parse("2006-01-02", in.Date); err != nil {
			return models.Transaction{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD", in.Date)
		}
		tx.Date = in.Date
	}

	if txType == "" {
		txType = classifyTx(tx)
	}

	accountID := tx.IncomeAccount
	if classifyTx(tx) == TxTypeTransfer && tx.OutcomeAccount != nil {
		accountID = *tx.OutcomeAccount
	}
	if in.AccountID != "" {
		accountID, err = resolveWritableAccountID(in.AccountID, env.maps)
		if err != nil {
			return models.Transaction{}, err
		}
	}

	toAccountID := tx.IncomeAccount
	if txType == TxTypeTransfer {
		if in.ToAccountID != "" {
			toAccountID, err = resolveWritableAccountID(in.ToAccountID, env.maps)
			if err != nil {
				return models.Transaction{}, err
			}
		} else if existing == nil {
			return models.Transaction{}, fmt.Errorf("to_account_id is required for transfers")
		}
		if toAccountID == "" {
			return models.Transaction{}, fmt.Errorf("to_account_id is required for transfers")
		}
	}

	instrumentID, _ := env.maps.AccountInstrument(accountID)
	if in.Currency != "" {
		instrumentID, err = resolveInstrumentRef(in.Currency, env.resp)
		if err != nil {
			return models.Transaction{}, err
		}
	}

	amount := majorAmount(tx)
	if in.Amount > 0 || existing == nil {
		amount = in.Amount
	}

	switch txType {
	case TxTypeExpense:
		tx.Income = 0
		tx.Outcome = amount
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = strPtr(accountID)
		tx.IncomeInstrument = instrumentID
		tx.OutcomeInstrument = instrumentID
	case TxTypeIncome:
		tx.Income = amount
		tx.Outcome = 0
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = strPtr(accountID)
		tx.IncomeInstrument = instrumentID
		tx.OutcomeInstrument = instrumentID
	case TxTypeTransfer:
		toInstrumentID, _ := env.maps.AccountInstrument(toAccountID)
		tx.Outcome = amount
		tx.Income = amount
		tx.OutcomeAccount = strPtr(accountID)
		tx.IncomeAccount = toAccountID
		tx.OutcomeInstrument = instrumentID
		tx.IncomeInstrument = toInstrumentID
	default:
		return models.Transaction{}, fmt.Errorf("unknown type %q: use expense, income, or transfer", txType)
	}

	if in.PayeeSet {
		tx.Payee = in.Payee
	}
	if in.CommentSet {
		tx.Comment = strPtr(in.Comment)
	}
	if in.ClearPayee {
		tx.Payee = ""
	}
	if in.ClearComment {
		tx.Comment = nil
	}

	switch {
	case in.ClearCategory:
		tx.Tag = nil
	case requestedExplicit:
		tx.Tag = requestedTagIDs
	}

	tx.Changed = int(time.Now().Unix())
	return tx, nil
}

func selectTransactionsByID(ids []string, env *transactionEnv) ([]models.Transaction, error) {
	selected := make([]models.Transaction, 0, len(ids))
	for _, id := range ids {
		tx, ok := env.txByID[id]
		if !ok {
			return nil, fmt.Errorf("transaction %q not found", id)
		}
		if tx.Deleted {
			continue
		}
		selected = append(selected, tx)
	}
	return selected, nil
}

type selectionFilter struct {
	AccountID     string
	CategoryID    string
	DateFrom      string
	DateTo        string
	Query         string
	Payee         string
	Type          TxType
	MinAmount     float64
	MaxAmount     float64
	Uncategorized bool
}

func filterTransactions(items []models.Transaction, filter selectionFilter) []models.Transaction {
	var out []models.Transaction
	for _, tx := range items {
		if tx.Deleted {
			continue
		}
		if filter.DateFrom != "" && tx.Date < filter.DateFrom {
			continue
		}
		if filter.DateTo != "" && tx.Date > filter.DateTo {
			continue
		}
		if filter.AccountID != "" && !transactionTouchesAccount(tx, filter.AccountID) {
			continue
		}
		if filter.CategoryID != "" && !containsString(tx.Tag, filter.CategoryID) {
			continue
		}
		if filter.Query != "" {
			comment := ""
			if tx.Comment != nil {
				comment = *tx.Comment
			}
			if !strings.Contains(strings.ToLower(tx.Payee), filter.Query) && !strings.Contains(strings.ToLower(comment), filter.Query) {
				continue
			}
		}
		if filter.Payee != "" && !strings.Contains(strings.ToLower(tx.Payee), filter.Payee) {
			continue
		}
		txType := classifyTx(tx)
		if filter.Uncategorized {
			if txType == TxTypeTransfer || len(tx.Tag) > 0 {
				continue
			}
		}
		if filter.Type != "" && txType != filter.Type {
			continue
		}
		amount := majorAmount(tx)
		if filter.MinAmount > 0 && amount < filter.MinAmount {
			continue
		}
		if filter.MaxAmount > 0 && amount > filter.MaxAmount {
			continue
		}
		out = append(out, tx)
	}
	return out
}

func resolveReadableAccountRef(ref string, maps runtime.LookupMaps) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("account is required")
	}
	if _, ok := maps.Accounts[ref]; ok {
		return ref, nil
	}
	for id, title := range maps.Accounts {
		if strings.EqualFold(title, ref) {
			return id, nil
		}
	}
	return "", fmt.Errorf("account %q not found", ref)
}

func resolveWritableAccountID(ref string, maps runtime.LookupMaps) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("account_id is required")
	}
	if _, ok := maps.Accounts[ref]; ok {
		return ref, nil
	}
	return "", fmt.Errorf("account %q not found by ID; use find_accounts to resolve the account ID first", ref)
}

func resolveCategoryRef(ref string, maps runtime.LookupMaps) (string, error) {
	return maps.ResolveTagRef(ref)
}

func resolveInstrumentRef(ref string, resp models.Response) (int, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, fmt.Errorf("currency is required")
	}
	if id, err := strconv.Atoi(ref); err == nil {
		for _, instr := range resp.Instrument {
			if instr.ID == id {
				return id, nil
			}
		}
	}
	for _, instr := range resp.Instrument {
		if strings.EqualFold(instr.Symbol, ref) || strings.EqualFold(instr.ShortTitle, ref) || strings.EqualFold(instr.Title, ref) {
			return instr.ID, nil
		}
	}
	return 0, fmt.Errorf("currency %q not found", ref)
}

func resolveRequestedTags(single string, many string, env *transactionEnv) ([]string, bool, error) {
	var refs []string
	if strings.TrimSpace(single) != "" {
		refs = append(refs, single)
	}
	if strings.TrimSpace(many) != "" {
		list, err := ParseFlexibleStringList(many)
		if err != nil {
			return nil, false, fmt.Errorf("invalid categories: %v", err)
		}
		refs = append(refs, list...)
	}
	if len(refs) == 0 {
		return nil, false, nil
	}

	tagIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		tagID, err := resolveCategoryRef(ref, env.maps)
		if err != nil {
			return nil, false, err
		}
		tagIDs = append(tagIDs, tagID)
	}
	sort.Strings(tagIDs)
	return uniqueStrings(tagIDs), true, nil
}

func resolveCategoriesForTransaction(ctx context.Context, c client.ZenClient, env *transactionEnv, tx models.Transaction, assisted bool) ([]string, string, string, error) {
	if !assisted {
		return nil, "", "", nil
	}

	historyMatches := make(map[string][]string)
	key := historicalCategorizationKey(tx)
	for _, candidate := range env.resp.Transaction {
		if candidate.Deleted || len(candidate.Tag) == 0 || classifyTx(candidate) != classifyTx(tx) {
			continue
		}
		if historicalCategorizationKey(candidate) != key {
			continue
		}
		sortedTags := append([]string(nil), candidate.Tag...)
		sort.Strings(sortedTags)
		historyMatches[strings.Join(sortedTags, ",")] = sortedTags
	}

	switch len(historyMatches) {
	case 1:
		for _, tags := range historyMatches {
			if len(tags) == 1 {
				return tags, "history", "", nil
			}
			return nil, "", "multiple categories found in historical match", nil
		}
	case 0:
	default:
		return nil, "", "conflicting historical categories", nil
	}

	suggested, err := c.Suggest(ctx, models.Transaction{
		Payee:   tx.Payee,
		Comment: tx.Comment,
	})
	if err != nil {
		return nil, "", fmt.Sprintf("suggest failed: %v", err), nil
	}
	if len(suggested.Tag) == 1 {
		if err := validateTagIDs(suggested.Tag, env.maps); err != nil {
			return nil, "", "", err
		}
		return suggested.Tag, "zenmoney_suggest", "", nil
	}
	if len(suggested.Tag) > 1 {
		return nil, "", "multiple suggested categories", nil
	}
	return nil, "", "", nil
}

func validateTagIDs(tagIDs []string, maps runtime.LookupMaps) error {
	for _, tagID := range tagIDs {
		if _, ok := maps.Tags[tagID]; !ok {
			return fmt.Errorf("tag %q not found", tagID)
		}
	}
	return nil
}

func pushRequest(serverTimestamp int, transactions []models.Transaction) models.Request {
	return models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        serverTimestamp,
		Transaction:            transactions,
	}
}

func shapeTransaction(tx models.Transaction, maps runtime.LookupMaps) TransactionResult {
	outcomeAcc := tx.IncomeAccount
	if tx.OutcomeAccount != nil {
		outcomeAcc = *tx.OutcomeAccount
	}

	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}

	tags := tx.Tag
	if tags == nil {
		tags = []string{}
	}

	return TransactionResult{
		ID:              tx.ID,
		Date:            tx.Date,
		Type:            classifyTx(tx),
		Income:          tx.Income,
		IncomeAccount:   maps.AccountName(tx.IncomeAccount),
		IncomeCurrency:  maps.InstrumentSymbol(tx.IncomeInstrument),
		Outcome:         tx.Outcome,
		OutcomeAccount:  maps.AccountName(outcomeAcc),
		OutcomeCurrency: maps.InstrumentSymbol(tx.OutcomeInstrument),
		Tags:            maps.TagNames(tags),
		Payee:           tx.Payee,
		Comment:         comment,
		Deleted:         tx.Deleted,
	}
}

func classifyTx(tx models.Transaction) TxType {
	if tx.Income > 0 && tx.Outcome > 0 {
		outcomeAcc := ""
		if tx.OutcomeAccount != nil {
			outcomeAcc = *tx.OutcomeAccount
		}
		if tx.IncomeAccount != outcomeAcc {
			return TxTypeTransfer
		}
	}
	if tx.Income > 0 {
		return TxTypeIncome
	}
	return TxTypeExpense
}

func transactionTouchesAccount(tx models.Transaction, accountID string) bool {
	return tx.IncomeAccount == accountID || (tx.OutcomeAccount != nil && *tx.OutcomeAccount == accountID)
}

func majorAmount(tx models.Transaction) float64 {
	if tx.Outcome > tx.Income {
		return tx.Outcome
	}
	return tx.Income
}

func normalizeText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(",", " ", ".", " ", ";", " ", ":", " ", "-", " ", "_", " ", "\t", " ", "\n", " ")
	s = replacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func historicalCategorizationKey(tx models.Transaction) string {
	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}
	return strings.Join([]string{string(classifyTx(tx)), normalizeText(tx.Payee), normalizeText(comment)}, "|")
}

// ParseFlexibleStringList parses a string that is either a JSON array or a
// comma/newline-separated list of values.
func ParseFlexibleStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var out []string
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, err
		}
		return compactStrings(out), nil
	}
	return compactStrings(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})), nil
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	out := items[:0]
	seen := map[string]struct{}{}
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
