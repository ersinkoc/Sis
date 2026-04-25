package policy

import (
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

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

type Decision struct {
	Blocked bool
	Reason  string
	List    string
}

type Policy struct {
	group       *Group
	lists       map[string]*Domains
	custom      *Domains
	allowlist   *Domains
	customAllow *Domains
	tz          *time.Location
}

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

func (e *Engine) ReplaceList(id string, domains *Domains) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if domains == nil {
		delete(e.lists, id)
		return
	}
	e.lists[id] = domains
}

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

func (e *Engine) AddCustomBlock(domain string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.custom.Add(domain)
}

func (e *Engine) RemoveCustomBlock(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.custom.Delete(domain)
}

func (e *Engine) AddCustomAllow(domain string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.customAllow.Add(domain)
}

func (e *Engine) RemoveCustomAllow(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.customAllow.Delete(domain)
}

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
	return nil
}

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
