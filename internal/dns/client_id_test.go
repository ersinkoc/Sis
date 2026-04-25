package dns

import (
	"net"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/store"
)

func TestClientIDResolveAndTouch(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	arp := NewARPTable(time.Minute)
	arp.entries.Store(map[string]string{"192.168.1.10": "aa:bb:cc:dd:ee:ff"})
	cid := NewClientID(arp, s.Clients())
	id := cid.Resolve(net.ParseIP("192.168.1.10"))
	if id.Key != "aa:bb:cc:dd:ee:ff" || id.Type != "mac" {
		t.Fatalf("identity = %#v", id)
	}
	if err := cid.Touch(id); err != nil {
		t.Fatal(err)
	}
	client, err := s.Clients().Get(id.Key)
	if err != nil {
		t.Fatal(err)
	}
	if client.Group != "default" || client.LastIP != "192.168.1.10" {
		t.Fatalf("client = %#v", client)
	}
}
