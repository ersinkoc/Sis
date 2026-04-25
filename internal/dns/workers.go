package dns

import (
	"context"
	"sync"
)

type workerPool struct {
	jobs chan func()
	wg   sync.WaitGroup
}

func newWorkerPool(ctx context.Context, n int, queue int) *workerPool {
	if n <= 0 {
		n = 1
	}
	if queue <= 0 {
		queue = n * 4
	}
	p := &workerPool{jobs: make(chan func(), queue)}
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-p.jobs:
					if !ok {
						return
					}
					job()
				}
			}
		}()
	}
	return p
}

func (p *workerPool) Submit(job func(), block bool) bool {
	if p == nil {
		go job()
		return true
	}
	if block {
		p.jobs <- job
		return true
	}
	select {
	case p.jobs <- job:
		return true
	default:
		return false
	}
}

func (p *workerPool) Close() {
	if p == nil {
		return
	}
	close(p.jobs)
	p.wg.Wait()
}
