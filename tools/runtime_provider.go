package tools

import (
	"context"
	"os"
	"sync"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/store"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// RuntimeProvider lazily initializes and caches the config and ZenMoney client.
// Failed initializations are retried on subsequent tool calls.
type RuntimeProvider struct {
	loadConfig   func() (*config.Config, error)
	newZenClient func(token string) (client.ZenClient, error)

	mu           sync.Mutex
	cfg          *config.Config
	token        string
	zenClientVal client.ZenClient
	syncSession  *syncSession

	// zenStore is always constructed at startup and is never nil.
	zenStore *store.Store

	importPlans struct {
		mu   sync.Mutex
		data map[string]*PreparedImportPlan
	}
}

// NewRuntimeProvider returns a RuntimeProvider ready for production use.
func NewRuntimeProvider() *RuntimeProvider {
	p := &RuntimeProvider{
		loadConfig: config.Load,
		newZenClient: func(token string) (client.ZenClient, error) {
			return client.New(token)
		},
		zenStore: store.New(store.DefaultPath()),
	}
	p.importPlans.data = make(map[string]*PreparedImportPlan)
	return p
}

// config returns the loaded and validated configuration, caching it after the first success.
func (p *RuntimeProvider) config() (*config.Config, error) {
	p.mu.Lock()
	if p.cfg != nil {
		cfg := p.cfg
		p.mu.Unlock()
		return cfg, nil
	}
	p.mu.Unlock()

	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.cfg == nil {
		p.cfg = cfg
	}
	cfg = p.cfg
	p.mu.Unlock()
	return cfg, nil
}

// apiClient returns the ZenMoney client, constructing it on first use.
func (p *RuntimeProvider) apiClient() (client.ZenClient, error) {
	p.mu.Lock()
	if p.zenClientVal != nil {
		c := p.zenClientVal
		p.mu.Unlock()
		return c, nil
	}
	p.mu.Unlock()

	if _, err := p.config(); err != nil {
		return nil, err
	}

	token := os.Getenv("ZENMONEY_TOKEN")
	c, err := p.newZenClient(token)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.zenClientVal == nil {
		p.zenClientVal = c
		p.token = token
	}
	c = p.zenClientVal
	p.mu.Unlock()
	return c, nil
}

func (p *RuntimeProvider) syncer() (*syncSession, error) {
	p.mu.Lock()
	if p.syncSession != nil {
		s := p.syncSession
		p.mu.Unlock()
		return s, nil
	}
	p.mu.Unlock()

	c, err := p.apiClient()
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.syncSession == nil {
		p.syncSession = newSyncSession(p.token, c, p.zenStore)
	}
	s := p.syncSession
	p.mu.Unlock()
	return s, nil
}

func (p *RuntimeProvider) incrementalSync(ctx context.Context) (models.Response, LookupMaps, error) {
	s, err := p.syncer()
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	resp, err := s.Incremental(ctx)
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	return resp, BuildLookupMaps(resp), nil
}

func (p *RuntimeProvider) scopedSync(ctx context.Context, scope []models.EntityType) (models.Response, LookupMaps, error) {
	s, err := p.syncer()
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	resp, err := s.Fetch(ctx, scope)
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	return resp, BuildLookupMaps(resp), nil
}

func (p *RuntimeProvider) fullSync(ctx context.Context) (models.Response, error) {
	s, err := p.syncer()
	if err != nil {
		return models.Response{}, err
	}
	return s.Full(ctx)
}

func (p *RuntimeProvider) currentServerTimestamp() int {
	s, err := p.syncer()
	if err != nil {
		return 0
	}
	return s.CurrentServerTimestamp()
}

func (p *RuntimeProvider) saveServerTimestamp(serverTimestamp int) {
	s, err := p.syncer()
	if err != nil {
		return
	}
	s.SaveServerTimestamp(serverTimestamp)
}

func (p *RuntimeProvider) storeImportPlan(id string, plan *PreparedImportPlan) {
	p.importPlans.mu.Lock()
	defer p.importPlans.mu.Unlock()
	if p.importPlans.data == nil {
		p.importPlans.data = make(map[string]*PreparedImportPlan)
	}
	p.importPlans.data[id] = plan
}

func (p *RuntimeProvider) takeImportPlan(id string) *PreparedImportPlan {
	p.importPlans.mu.Lock()
	defer p.importPlans.mu.Unlock()
	if p.importPlans.data == nil {
		return nil
	}
	plan, ok := p.importPlans.data[id]
	if !ok {
		return nil
	}
	delete(p.importPlans.data, id)
	return plan
}
