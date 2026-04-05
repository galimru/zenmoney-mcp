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

	// preparations holds in-memory bulk operation sets keyed by UUID.
	preparations struct {
		mu   sync.Mutex
		data map[string]*PreparedBulk
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
	p.preparations.data = make(map[string]*PreparedBulk)
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

// storePreparation stores a prepared bulk operation under the given ID.
func (p *RuntimeProvider) storePreparation(id string, bulk *PreparedBulk) {
	p.preparations.mu.Lock()
	defer p.preparations.mu.Unlock()
	p.preparations.data[id] = bulk
}

// takePreparation retrieves and removes a prepared bulk operation.
// Returns nil if not found.
func (p *RuntimeProvider) takePreparation(id string) *PreparedBulk {
	p.preparations.mu.Lock()
	defer p.preparations.mu.Unlock()
	bulk, ok := p.preparations.data[id]
	if !ok {
		return nil
	}
	delete(p.preparations.data, id)
	return bulk
}
