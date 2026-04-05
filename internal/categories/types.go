package categories

type FindInput struct {
	Query string
	Limit int
}

type AddInput struct {
	Title          string
	ParentCategory string
	Icon           string
	Color          *int64
	ShowIncome     bool
	ShowOutcome    bool
	BudgetIncome   bool
	BudgetOutcome  bool
	Required       *bool
}

type CategoryResult struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Parent *string `json:"parent,omitempty"`
}
