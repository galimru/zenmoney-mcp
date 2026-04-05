package runtime

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/store"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type Provider struct {
	loadConfig   func() (*config.Config, error)
	newZenClient func(token string) (client.ZenClient, error)
	store        *store.Store
	logger       *slog.Logger

	mu      sync.Mutex
	cfg     *config.Config
	token   string
	client  client.ZenClient
	session *Session
}

func NewProvider() *Provider {
	return NewProviderWithDeps(config.Load, func(token string) (client.ZenClient, error) {
		return client.New(token)
	}, store.New(store.DefaultPath()), nil)
}

func NewProviderWithDeps(
	loadConfig func() (*config.Config, error),
	newZenClient func(token string) (client.ZenClient, error),
	syncStore *store.Store,
	logger *slog.Logger,
) *Provider {
	return &Provider{
		loadConfig:   loadConfig,
		newZenClient: newZenClient,
		store:        syncStore,
		logger:       logger,
	}
}

func (p *Provider) Config() (*config.Config, error) {
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

func (p *Provider) Client() (client.ZenClient, error) {
	p.mu.Lock()
	if p.client != nil {
		c := p.client
		p.mu.Unlock()
		return c, nil
	}
	p.mu.Unlock()

	if _, err := p.Config(); err != nil {
		return nil, err
	}

	token := os.Getenv("ZENMONEY_TOKEN")
	c, err := p.newZenClient(token)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.client == nil {
		p.client = c
		p.token = token
	}
	c = p.client
	p.mu.Unlock()
	return c, nil
}

func (p *Provider) ScopedSync(ctx context.Context, scope []models.EntityType) (models.Response, LookupMaps, error) {
	s, err := p.sessionForUse()
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	resp, err := s.Fetch(ctx, scope)
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	return resp, BuildLookupMaps(resp), nil
}

func (p *Provider) IncrementalSync(ctx context.Context) (models.Response, LookupMaps, error) {
	s, err := p.sessionForUse()
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	resp, err := s.Incremental(ctx)
	if err != nil {
		return models.Response{}, LookupMaps{}, err
	}
	return resp, BuildLookupMaps(resp), nil
}

func (p *Provider) FullSync(ctx context.Context) (models.Response, error) {
	s, err := p.sessionForUse()
	if err != nil {
		return models.Response{}, err
	}
	return s.Full(ctx)
}

func (p *Provider) CurrentServerTimestamp() int {
	s, err := p.sessionForUse()
	if err != nil {
		return 0
	}
	return s.CurrentServerTimestamp()
}

func (p *Provider) SaveServerTimestamp(serverTimestamp int) error {
	s, err := p.sessionForUse()
	if err != nil {
		return err
	}
	return s.SaveServerTimestamp(serverTimestamp)
}

func (p *Provider) sessionForUse() (*Session, error) {
	p.mu.Lock()
	if p.session != nil {
		s := p.session
		p.mu.Unlock()
		return s, nil
	}
	p.mu.Unlock()

	c, err := p.Client()
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.session == nil {
		p.session = NewSession(p.token, c, p.store, p.logger)
	}
	s := p.session
	p.mu.Unlock()
	return s, nil
}
