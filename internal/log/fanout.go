package log

import "sync"

type Subscription chan Entry

type fanout struct {
	mu     sync.Mutex
	subs   map[Subscription]struct{}
	ring   []Entry
	next   int
	filled bool
}

func newFanout(ringSize int) *fanout {
	if ringSize < 0 {
		ringSize = 0
	}
	return &fanout{subs: make(map[Subscription]struct{}), ring: make([]Entry, ringSize)}
}

func (f *fanout) subscribe(size int, replay bool) Subscription {
	if size <= 0 {
		size = 1
	}
	sub := make(Subscription, size)
	f.mu.Lock()
	f.subs[sub] = struct{}{}
	var backlog []Entry
	if replay {
		backlog = f.snapshotLocked()
	}
	f.mu.Unlock()
	for _, e := range backlog {
		dropOldestSend(sub, e)
	}
	return sub
}

func (f *fanout) unsubscribe(sub Subscription) {
	f.mu.Lock()
	if _, ok := f.subs[sub]; ok {
		delete(f.subs, sub)
		close(sub)
	}
	f.mu.Unlock()
}

func (f *fanout) publish(e Entry) {
	f.mu.Lock()
	if len(f.ring) > 0 {
		f.ring[f.next] = e.clone()
		f.next = (f.next + 1) % len(f.ring)
		if f.next == 0 {
			f.filled = true
		}
	}
	for sub := range f.subs {
		dropOldestSend(sub, e)
	}
	f.mu.Unlock()
}

func (f *fanout) snapshotLocked() []Entry {
	if len(f.ring) == 0 || (!f.filled && f.next == 0) {
		return nil
	}
	var out []Entry
	if !f.filled {
		out = append(out, f.ring[:f.next]...)
	} else {
		out = append(out, f.ring[f.next:]...)
		out = append(out, f.ring[:f.next]...)
	}
	for i := range out {
		out[i] = out[i].clone()
	}
	return out
}

func (f *fanout) snapshot() []Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshotLocked()
}

func dropOldestSend(ch chan Entry, e Entry) {
	select {
	case ch <- e.clone():
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- e.clone():
	default:
	}
}
