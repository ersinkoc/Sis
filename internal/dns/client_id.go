package dns

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/store"
)

type Identity struct {
	Key  string
	Type string
	IP   net.IP
}

type ClientID struct {
	arp      *ARPTable
	clients  store.ClientStore
	mu       sync.Mutex
	touched  map[string]time.Time
	debounce time.Duration
	now      func() time.Time
}

func NewClientID(arp *ARPTable, clients store.ClientStore) *ClientID {
	if arp == nil {
		arp = NewARPTable(30 * time.Second)
	}
	return &ClientID{
		arp: arp, clients: clients, touched: make(map[string]time.Time),
		debounce: time.Minute, now: time.Now,
	}
}

func (c *ClientID) Resolve(ip net.IP) Identity {
	if c == nil || ip == nil {
		return Identity{}
	}
	if mac, ok := c.arp.Lookup(ip); ok {
		return Identity{Key: mac, Type: "mac", IP: ip}
	}
	return Identity{Key: ip.String(), Type: "ip", IP: ip}
}

func (c *ClientID) Touch(id Identity) error {
	if c == nil || c.clients == nil || id.Key == "" {
		return nil
	}
	now := c.now()
	c.mu.Lock()
	if last := c.touched[id.Key]; !last.IsZero() && now.Sub(last) < c.debounce {
		c.mu.Unlock()
		return nil
	}
	c.touched[id.Key] = now
	c.mu.Unlock()

	client, err := c.clients.Get(id.Key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if client == nil {
		client = &store.Client{
			Key: id.Key, Type: id.Type, Group: "default",
			FirstSeen: now, LastSeen: now, LastIP: id.IP.String(),
		}
		return c.clients.Upsert(client)
	}
	client.LastSeen = now
	client.LastIP = id.IP.String()
	if client.Group == "" {
		client.Group = "default"
	}
	if client.Type == "" {
		client.Type = id.Type
	}
	return c.clients.Upsert(client)
}
