package policy

import (
	"testing"

	"github.com/ersinkoc/sis/internal/store"
)

func TestStoreClientResolver(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Clients().Upsert(&store.Client{Key: "kid", Group: "kids"}); err != nil {
		t.Fatal(err)
	}
	resolver := StoreClientResolver{Clients: s.Clients()}
	if got := resolver.GroupOf("kid"); got != "kids" {
		t.Fatalf("GroupOf(kid) = %q", got)
	}
	if got := resolver.GroupOf("missing"); got != "default" {
		t.Fatalf("GroupOf(missing) = %q", got)
	}
}
