package upstream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

// Pool holds configured DoH clients and forwards through healthy upstreams.
type Pool struct {
	mu           sync.RWMutex
	clients      []*pooledClient
	probeInterval time.Duration
}

type pooledClient struct {
	client         *DoHClient
	healthy        bool
	errors         int
	threshold      int
	probeTimeout   time.Duration
}

// Attempt records the outcome of one upstream forwarding attempt.
type Attempt struct {
	ID      string
	OK      bool
	Healthy bool
}

// NewPool creates a forwarding pool from upstream configs.
func NewPool(upstreams []config.Upstream) *Pool {
	p := &Pool{}
	p.Replace(upstreams)
	return p
}

// Replace atomically replaces all configured upstream clients.
func (p *Pool) Replace(upstreams []config.Upstream) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients = nil
	p.probeInterval = time.Minute // default
	for _, upstream := range upstreams {
		threshold := 3
		if upstream.CircuitBreakerThreshold > 0 {
			threshold = upstream.CircuitBreakerThreshold
		}
		probeTimeout := 10 * time.Second
		if upstream.HealthProbeTimeout.Duration > 0 {
			probeTimeout = upstream.HealthProbeTimeout.Duration
		}
		if upstream.HealthProbeInterval.Duration > 0 && p.probeInterval <= 0 {
			p.probeInterval = upstream.HealthProbeInterval.Duration
		}
		p.clients = append(p.clients, &pooledClient{
			client:       NewDoHClient(upstream),
			healthy:       true,
			threshold:     threshold,
			probeTimeout:  probeTimeout,
		})
	}
}

// Forward tries each healthy upstream until one returns a DNS response.
// It retries failed upstreams with exponential backoff (maxRetries per upstream).
func (p *Pool) Forward(ctx context.Context, msg *mdns.Msg) (*mdns.Msg, string, []Attempt, error) {
	if p == nil {
		return nil, "", nil, fmt.Errorf("upstream pool is not configured")
	}
	if msg == nil {
		return nil, "", nil, fmt.Errorf("dns message is required")
	}
	p.mu.RLock()
	var candidates []*DoHClient
	for _, candidate := range p.clients {
		if candidate.healthy {
			candidates = append(candidates, candidate.client)
		}
	}
	p.mu.RUnlock()

	const maxRetries = 2
	const baseDelay = 50 * time.Millisecond
	attempts := make([]Attempt, 0, len(candidates)*(maxRetries+1))
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
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
		if attempt < maxRetries {
			delay := baseDelay * time.Duration(1<<attempt)
			select {
			case <-ctx.Done():
				return nil, "", attempts, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no healthy upstreams")
	}
	return nil, "", attempts, lastErr
}

// Test probes a configured upstream by ID and updates its health state.
func (p *Pool) Test(ctx context.Context, id string) (*mdns.Msg, error) {
	if p == nil {
		return nil, fmt.Errorf("upstream pool is not configured")
	}
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

// RunHealthProber periodically probes unhealthy upstreams until ctx is canceled.
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

// ProbeUnhealthy probes currently unhealthy upstreams once.
func (p *Pool) ProbeUnhealthy(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.RLock()
	var probs []struct {
		client     *DoHClient
		probeTimeout time.Duration
	}
	for _, pc := range p.clients {
		if !pc.healthy {
			probs = append(probs, struct {
				client     *DoHClient
				probeTimeout time.Duration
			}{client: pc.client, probeTimeout: pc.probeTimeout})
		}
	}
	p.mu.RUnlock()
	for _, pr := range probs {
		probeCtx, cancel := context.WithTimeout(ctx, pr.probeTimeout)
		_, err := probeClient(probeCtx, pr.client)
		cancel()
		if err == nil {
			p.markSuccess(pr.client.ID())
		}
	}
}

func probeClient(ctx context.Context, client *DoHClient) (*mdns.Msg, error) {
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	return client.Forward(ctx, msg)
}

func (p *Pool) markSuccess(id string) {
	if p == nil {
		return
	}
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
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, candidate := range p.clients {
		if candidate.client.ID() == id {
			candidate.errors++
			if candidate.errors >= candidate.threshold {
				candidate.healthy = false
			}
			return
		}
	}
}

// HealthyIDs returns IDs for upstreams currently marked healthy.
func (p *Pool) HealthyIDs() []string {
	if p == nil {
		return nil
	}
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

// AllIDs returns IDs for every configured upstream.
func (p *Pool) AllIDs() []string {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.clients))
	for _, candidate := range p.clients {
		out = append(out, candidate.client.ID())
	}
	return out
}

// IsHealthy reports whether the upstream id is currently marked healthy.
func (p *Pool) IsHealthy(id string) bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, candidate := range p.clients {
		if candidate.client.ID() == id {
			return candidate.healthy
		}
	}
	return false
}

// ProbeInterval returns the configured health probe interval.
func (p *Pool) ProbeInterval() time.Duration {
	if p == nil {
		return time.Minute
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.probeInterval <= 0 {
		return time.Minute
	}
	return p.probeInterval
}
