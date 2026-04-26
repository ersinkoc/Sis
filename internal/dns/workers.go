package dns

import (
	"context"
	"sync"
)

type workerPool struct {
	jobs      chan func()
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newWorkerPool(ctx context.Context, n int, queue int) *workerPool {
	if ctx == nil {
		ctx = context.Background()
	}
	if n <= 0 {
		n = 1
	}
	if queue <= 0 {
		queue = n * 4
	}
	p := &workerPool{jobs: make(chan func(), queue), done: make(chan struct{})}
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-p.done:
					return
				case job, ok := <-p.jobs:
					if !ok {
						return
					}
					if job == nil {
						continue
					}
					job()
				}
			}
		}()
	}
	return p
}

func (p *workerPool) Submit(job func(), block bool) bool {
	if job == nil {
		return false
	}
	if p == nil {
		go job()
		return true
	}
	select {
	case <-p.done:
		return false
	default:
	}
	if block {
		select {
		case p.jobs <- job:
			return true
		case <-p.done:
			return false
		}
	}
	select {
	case p.jobs <- job:
		return true
	case <-p.done:
		return false
	default:
		return false
	}
}

func (p *workerPool) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		close(p.done)
	})
	p.wg.Wait()
}
