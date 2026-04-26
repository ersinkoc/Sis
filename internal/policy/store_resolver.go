package policy

import "github.com/ersinkoc/sis/internal/store"

// StoreClientResolver resolves client groups from a ClientStore.
type StoreClientResolver struct {
	Clients store.ClientStore
}

// GroupOf returns the stored group for clientKey or default.
func (r StoreClientResolver) GroupOf(clientKey string) string {
	if r.Clients == nil || clientKey == "" {
		return "default"
	}
	client, err := r.Clients.Get(clientKey)
	if err != nil || client == nil || client.Group == "" {
		return "default"
	}
	return client.Group
}
