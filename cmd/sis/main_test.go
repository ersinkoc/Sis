package main

import (
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/store"
)

func TestSeedConfigClientsCreatesAndUpdates(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Clients: []config.Client{{
		Key: "192.168.1.50", Type: "ip", Name: "TV", Group: "default", Hidden: true,
	}}}
	if err := seedConfigClients(st, cfg); err != nil {
		t.Fatal(err)
	}
	client, err := st.Clients().Get("192.168.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "TV" || client.Group != "default" || !client.Hidden {
		t.Fatalf("client = %#v", client)
	}
	firstSeen := client.FirstSeen

	client.LastSeen = time.Now().UTC().Add(time.Hour)
	if err := st.Clients().Upsert(client); err != nil {
		t.Fatal(err)
	}
	cfg.Clients[0].Name = "Living Room TV"
	cfg.Clients[0].Hidden = false
	if err := seedConfigClients(st, cfg); err != nil {
		t.Fatal(err)
	}
	client, err = st.Clients().Get("192.168.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "Living Room TV" || client.Hidden {
		t.Fatalf("client was not updated: %#v", client)
	}
	if !client.FirstSeen.Equal(firstSeen) {
		t.Fatalf("first_seen changed: before=%s after=%s", firstSeen, client.FirstSeen)
	}
}

func TestSeedConfigClientsDefaults(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := seedConfigClients(st, &config.Config{Clients: []config.Client{{Key: "192.168.1.51"}}}); err != nil {
		t.Fatal(err)
	}
	client, err := st.Clients().Get("192.168.1.51")
	if err != nil {
		t.Fatal(err)
	}
	if client.Type != "ip" || client.Group != "default" {
		t.Fatalf("client defaults = %#v", client)
	}
}
