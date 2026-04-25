package dns

import "testing"

func TestServerTCPSlots(t *testing.T) {
	s := &Server{tcpSlots: make(chan struct{}, 1)}
	if !s.acquireTCPSlot() {
		t.Fatal("first slot should be acquired")
	}
	if s.acquireTCPSlot() {
		t.Fatal("second slot should be rejected")
	}
	s.releaseTCPSlot()
	if !s.acquireTCPSlot() {
		t.Fatal("slot should be available after release")
	}
}
