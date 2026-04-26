package policy

import (
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

// Engine owns compiled policy state and supports lock-free snapshots for evaluation.
type Engine struct {
	mu          sync.RWMutex
	groups      map[string]*Group
	lists       map[string]*Domains
	custom      *Domains
	allowlist   *Domains
	customAllow *Domains
	clients     ClientResolver
	tz          *time.Location
}

// Decision is the result of evaluating one DNS query against policy.
type Decision struct {
	Blocked bool
	Reason  string
	List    string
}

// Policy is an immutable evaluation snapshot for one client identity.
type Policy struct {
	group       *Group
	lists       map[string]*Domains
	custom      *Domains
	allowlist   *Domains
	customAllow *Domains
	tz          *time.Location
}

// NewEngine compiles config policy and creates an Engine.
func NewEngine(c *config.Config, clients ClientResolver) (*Engine, error) {
	if clients == nil {
		clients = StaticClientResolver{}
	}
	groups, err := CompileGroups(c.Groups)
	if err != nil {
		return nil, err
	}
	allowlist := NewDomains()
	for _, domain := range c.Allowlist.Domains {
		if !allowlist.Add(domain) {
			return nil, errInvalidDomain("allowlist.domains", domain)
		}
	}
	tz := time.Local
	if c.Server.TZ != "" && c.Server.TZ != "Local" {
		loaded, err := time.LoadLocation(c.Server.TZ)
		if err != nil {
			return nil, err
		}
		tz = loaded
	}
	return &Engine{
		groups: groups, lists: make(map[string]*Domains),
		custom: NewDomains(), allowlist: allowlist, customAllow: NewDomains(),
		clients: clients, tz: tz,
	}, nil
}

// For returns a policy snapshot for id using the configured client resolver.
func (e *Engine) For(id Identity) *Policy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	groupName := "default"
	if e.clients != nil {
		if resolved := e.clients.GroupOf(id.Key); resolved != "" {
			groupName = resolved
		}
	}
	group := e.groups[groupName]
	if group == nil {
		group = e.groups["default"]
	}
	lists := make(map[string]*Domains, len(e.lists))
	for id, domains := range e.lists {
		lists[id] = domains
	}
	return &Policy{
		group: group, lists: lists, custom: e.custom,
		allowlist: e.allowlist, customAllow: e.customAllow, tz: e.tz,
	}
}

// ReplaceList swaps one compiled blocklist into the engine.
func (e *Engine) ReplaceList(id string, domains *Domains) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if domains == nil {
		delete(e.lists, id)
		return
	}
	e.lists[id] = domains
}

// ListEntries returns searchable entries for a compiled blocklist.
func (e *Engine) ListEntries(id, query string, limit int) ([]string, bool) {
	if e == nil {
		return nil, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	domains := e.lists[id]
	if domains == nil {
		return nil, false
	}
	return domains.Entries(query, limit), true
}

// AddCustomBlock adds a domain to the runtime custom blocklist.
func (e *Engine) AddCustomBlock(domain string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.custom.Add(domain)
}

// RemoveCustomBlock removes a domain from the runtime custom blocklist.
func (e *Engine) RemoveCustomBlock(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.custom.Delete(domain)
}

// AddCustomAllow adds a domain to the runtime custom allowlist.
func (e *Engine) AddCustomAllow(domain string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.customAllow.Add(domain)
}

// RemoveCustomAllow removes a domain from the runtime custom allowlist.
func (e *Engine) RemoveCustomAllow(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.customAllow.Delete(domain)
}

// ReloadConfig recompiles config-derived policy while preserving downloaded and custom lists.
func (e *Engine) ReloadConfig(c *config.Config) error {
	groups, err := CompileGroups(c.Groups)
	if err != nil {
		return err
	}
	allowlist := NewDomains()
	for _, domain := range c.Allowlist.Domains {
		if !allowlist.Add(domain) {
			return errInvalidDomain("allowlist.domains", domain)
		}
	}
	tz := time.Local
	if c.Server.TZ != "" && c.Server.TZ != "Local" {
		loaded, err := time.LoadLocation(c.Server.TZ)
		if err != nil {
			return err
		}
		tz = loaded
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.groups = groups
	e.allowlist = allowlist
	e.tz = tz
	e.removeDisabledListsLocked(c)
	return nil
}

// Evaluate returns the policy decision for qname at now.
func (p *Policy) Evaluate(qname string, _ uint16, now time.Time) Decision {
	if p == nil || p.group == nil {
		return Decision{}
	}
	if p.allowlist.Match(qname) || p.customAllow.Match(qname) || p.group.AllowlistTree.Match(qname) {
		return Decision{}
	}
	for _, listID := range p.group.BaseLists {
		if p.matchList(listID, qname) {
			return Decision{Blocked: true, Reason: "blocklist:" + listID, List: listID}
		}
	}
	for _, schedule := range p.group.Schedules {
		if !schedule.ActiveAt(now, p.tz) {
			continue
		}
		for _, listID := range schedule.Block {
			if p.matchList(listID, qname) {
				return Decision{Blocked: true, Reason: "schedule:" + schedule.Name, List: listID}
			}
		}
	}
	if p.custom.Match(qname) {
		return Decision{Blocked: true, Reason: "blocklist:custom", List: "custom"}
	}
	return Decision{}
}

func (p *Policy) matchList(id, qname string) bool {
	domains := p.lists[id]
	return domains != nil && domains.Match(qname)
}

func (e *Engine) removeDisabledListsLocked(c *config.Config) {
	enabled := make(map[string]struct{}, len(c.Blocklists))
	for _, list := range c.Blocklists {
		if list.Enabled {
			enabled[list.ID] = struct{}{}
		}
	}
	for id := range e.lists {
		if _, ok := enabled[id]; !ok {
			delete(e.lists, id)
		}
	}
}
