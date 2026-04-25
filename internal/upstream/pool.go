package upstream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

type Pool struct {
	mu      sync.RWMutex
	clients []*pooledClient
}

type pooledClient struct {
	client  *DoHClient
	healthy bool
	errors  int
}

type Attempt struct {
	ID      string
	OK      bool
	Healthy bool
}

func NewPool(upstreams []config.Upstream) *Pool {
	p := &Pool{}
	p.Replace(upstreams)
	return p
}

func (p *Pool) Replace(upstreams []config.Upstream) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients = nil
	for _, upstream := range upstreams {
		p.clients = append(p.clients, &pooledClient{client: NewDoHClient(upstream), healthy: true})
	}
}

func (p *Pool) Forward(ctx context.Context, msg *mdns.Msg) (*mdns.Msg, string, []Attempt, error) {
	p.mu.RLock()
	var candidates []*DoHClient
	for _, candidate := range p.clients {
		if candidate.healthy {
			candidates = append(candidates, candidate.client)
		}
	}
	p.mu.RUnlock()
	var lastErr error
	attempts := make([]Attempt, 0, len(candidates))
	for _, candidate := range candidates {
		resp, err := candidate.Forward(ctx, msg)
		if err == nil {
			p.markSuccess(candidate.ID())
			attempts = append(attempts, Attempt{ID: candidate.ID(), OK: true, Healthy: true})
			return resp, candidate.ID(), attempts, nil
		}
		lastErr = err
		p.markFailure(candidate.ID())
		attempts = append(attempts, Attempt{ID: candidate.ID(), OK: false, Healthy: p.IsHealthy(candidate.ID())})
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no healthy upstreams")
	}
	return nil, "", attempts, lastErr
}

func (p *Pool) Test(ctx context.Context, id string) (*mdns.Msg, error) {
	p.mu.RLock()
	var client *DoHClient
	for _, candidate := range p.clients {
		if candidate.client.ID() == id {
			client = candidate.client
			break
		}
	}
	p.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("upstream %q not found", id)
	}
	resp, err := probeClient(ctx, client)
	if err != nil {
		p.markFailure(id)
		return nil, err
	}
	p.markSuccess(id)
	return resp, nil
}

func (p *Pool) RunHealthProber(ctx context.Context, interval time.Duration) {
	if p == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.ProbeUnhealthy(ctx)
		}
	}
}

func (p *Pool) ProbeUnhealthy(ctx context.Context) {
	for _, client := range p.unhealthyClients() {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := probeClient(probeCtx, client)
		cancel()
		if err == nil {
			p.markSuccess(client.ID())
		}
	}
}

func (p *Pool) unhealthyClients() []*DoHClient {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*DoHClient, 0)
	for _, candidate := range p.clients {
		if !candidate.healthy {
			out = append(out, candidate.client)
		}
	}
	return out
}

func probeClient(ctx context.Context, client *DoHClient) (*mdns.Msg, error) {
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	return client.Forward(ctx, msg)
}

func (p *Pool) markSuccess(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, candidate := range p.clients {
		if candidate.client.ID() == id {
			candidate.errors = 0
			candidate.healthy = true
			return
		}
	}
}

func (p *Pool) markFailure(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, candidate := range p.clients {
		if candidate.client.ID() == id {
			candidate.errors++
			if candidate.errors >= 3 {
				candidate.healthy = false
			}
			return
		}
	}
}

func (p *Pool) HealthyIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []string
	for _, candidate := range p.clients {
		if candidate.healthy {
			out = append(out, candidate.client.ID())
		}
	}
	return out
}

func (p *Pool) AllIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.clients))
	for _, candidate := range p.clients {
		out = append(out, candidate.client.ID())
	}
	return out
}

func (p *Pool) IsHealthy(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, candidate := range p.clients {
		if candidate.client.ID() == id {
			return candidate.healthy
		}
	}
	return false
}
