package config

import (
	"sync"
	"testing"
)

func TestHolderConcurrentGetReplace(t *testing.T) {
	h := NewHolder(&Config{Server: Server{DataDir: "one"}})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if h.Get() == nil {
					t.Error("holder returned nil")
					return
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		h.Replace(&Config{Server: Server{DataDir: "two"}})
	}
	wg.Wait()
}
