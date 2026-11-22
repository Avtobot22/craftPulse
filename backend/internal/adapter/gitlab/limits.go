package gitlab

import (
	"context"
	"sync"
)

type requestLimits struct {
	mu      sync.Mutex
	entries map[string]*requestLimit
}

type requestLimit struct {
	slots chan struct{}
	calls int
}

func (l *requestLimits) share(key string, capacity int) (limit *requestLimit, release func()) {
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*requestLimit)
	}
	limit = l.entries[key]
	if limit == nil {
		limit = &requestLimit{slots: make(chan struct{}, capacity)}
		l.entries[key] = limit
	}
	limit.calls++
	l.mu.Unlock()

	return limit, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		limit.calls--
		if limit.calls == 0 {
			delete(l.entries, key)
		}
	}
}

func (l *requestLimit) acquire(ctx context.Context) error {
	select {
	case l.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			l.release()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *requestLimit) release() { <-l.slots }
