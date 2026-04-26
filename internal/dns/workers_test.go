package dns

import (
	"context"
	"testing"
	"time"
)

func TestWorkerPoolRejectsNilJob(t *testing.T) {
	pool := newWorkerPool(context.Background(), 1, 1)
	defer pool.Close()
	if pool.Submit(nil, false) {
		t.Fatal("nil job should be rejected")
	}
}

func TestWorkerPoolNilFallbackRejectsNilJob(t *testing.T) {
	var pool *workerPool
	if pool.Submit(nil, false) {
		t.Fatal("nil pool should reject nil job")
	}
}

func TestWorkerPoolCloseIsIdempotent(t *testing.T) {
	pool := newWorkerPool(context.Background(), 1, 1)
	pool.Close()
	pool.Close()
	if pool.Submit(func() {}, false) {
		t.Fatal("submit after close should fail")
	}
}

func TestWorkerPoolAcceptsNilContext(t *testing.T) {
	pool := newWorkerPool(nil, 1, 1)
	defer pool.Close()
	done := make(chan struct{})
	if !pool.Submit(func() { close(done) }, true) {
		t.Fatal("submit failed")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("job did not run")
	}
}
