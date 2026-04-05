package tools

import (
	"os"
	"sync"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/store"
)

// RuntimeProvider lazily initializes and caches the config and ZenMoney client.
// Failed initializations are retried on subsequent tool calls.
type RuntimeProvider struct {
	loadConfig   func() (*config.Config, error)
	newZenClient func(token string) (client.ZenClient, error)

	mu           sync.Mutex
	cfg          *config.Config
	zenClientVal client.ZenClient

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

	c, err := p.newZenClient(os.Getenv("ZENMONEY_TOKEN"))
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.zenClientVal == nil {
		p.zenClientVal = c
	}
	c = p.zenClientVal
	p.mu.Unlock()
	return c, nil
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
