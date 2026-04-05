package categories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/google/uuid"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// Runtime is the narrow subset of Provider that this service needs.
type Runtime interface {
	Client() (client.ZenClient, error)
	ScopedSync(ctx context.Context, scope []models.EntityType) (models.Response, runtime.LookupMaps, error)
	CurrentServerTimestamp() int
	SaveServerTimestamp(ts int) error
}

type Service struct{ runtime Runtime }

func NewService(rt Runtime) *Service { return &Service{runtime: rt} }

func (s *Service) Find(ctx context.Context, in FindInput) ([]CategoryResult, error) {
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 100 {
		in.Limit = 100
	}

	resp, maps, err := s.runtime.ScopedSync(ctx, runtime.ScopeTags)
	if err != nil {
		return nil, err
	}

	query := strings.ToLower(strings.TrimSpace(in.Query))
	results := make([]CategoryResult, 0, in.Limit)
	for _, tag := range resp.Tag {
		if query != "" && !strings.Contains(strings.ToLower(tag.Title), query) {
			continue
		}
		results = append(results, toCategoryResult(tag, maps))
		if len(results) >= in.Limit {
			break
		}
	}
	return results, nil
}

func (s *Service) Add(ctx context.Context, in AddInput) (CategoryResult, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return CategoryResult{}, fmt.Errorf("title is required and must not be empty")
	}

	c, err := s.runtime.Client()
	if err != nil {
		return CategoryResult{}, err
	}

	resp, maps, err := s.runtime.ScopedSync(ctx, runtime.ScopeTagsWithUser)
	if err != nil {
		return CategoryResult{}, err
	}

	// Idempotent: return existing category if title already exists.
	for _, tag := range resp.Tag {
		if strings.EqualFold(tag.Title, title) {
			return toCategoryResult(tag, maps), nil
		}
	}

	userID := 0
	if len(resp.User) > 0 {
		userID = resp.User[0].ID
	}

	var parentPtr *string
	if in.ParentCategory != "" {
		parentID, err := maps.ResolveTagRef(in.ParentCategory)
		if err != nil {
			return CategoryResult{}, err
		}
		parentPtr = &parentID
	}

	var iconPtr *string
	if in.Icon != "" {
		iconPtr = &in.Icon
	}

	newTag := models.Tag{
		ID:            uuid.New().String(),
		User:          userID,
		Changed:       int(time.Now().Unix()),
		Title:         title,
		Icon:          iconPtr,
		Color:         in.Color,
		Parent:        parentPtr,
		ShowIncome:    in.ShowIncome,
		ShowOutcome:   in.ShowOutcome,
		BudgetIncome:  in.BudgetIncome,
		BudgetOutcome: in.BudgetOutcome,
		Required:      in.Required,
	}

	pushResp, err := c.Push(ctx, buildPushRequest(s.runtime.CurrentServerTimestamp(), []models.Tag{newTag}))
	if err != nil {
		return CategoryResult{}, fmt.Errorf("create category: %w", err)
	}
	if pushResp.ServerTimestamp > 0 {
		_ = s.runtime.SaveServerTimestamp(pushResp.ServerTimestamp)
	}

	maps.Tags[newTag.ID] = newTag.Title
	return toCategoryResult(newTag, maps), nil
}

func toCategoryResult(tag models.Tag, maps runtime.LookupMaps) CategoryResult {
	var parent *string
	if tag.Parent != nil && *tag.Parent != "" {
		name := maps.Tags[*tag.Parent]
		if name == "" {
			name = *tag.Parent
		}
		parent = &name
	}
	return CategoryResult{
		ID:     tag.ID,
		Title:  tag.Title,
		Parent: parent,
	}
}

func buildPushRequest(serverTimestamp int, tags []models.Tag) models.Request {
	return models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        serverTimestamp,
		Tag:                    tags,
	}
}
