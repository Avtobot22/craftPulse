package gitadapter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const _sizeEvery = 250 * time.Millisecond

var (
	errCloneTooLarge = errors.New("gitadapter: clone exceeds the size limit")
	errCloneTimeout  = errors.New("gitadapter: clone exceeds the time limit")
)

// clones keeps one bare partial clone per repository of a connection on the disk of the container, at
// most maxBytes in all: the least recently used clones nobody reads now are removed first (TZ 14.9). A
// clone runs in the background, so a call on a large repository answers preparing instead of waiting.
type clones struct {
	root     string
	maxBytes int64
	cloneMax int64
	timeout  time.Duration
	hold     time.Duration
	now      func() time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	items    map[string]*clone
}

type clone struct {
	key string
	dir string
	// mu lets one command change the clone at a time; reads of a ready clone share it.
	mu       sync.RWMutex
	done     chan struct{}
	err      error
	failedAt time.Time
	size     int64
	used     time.Time
	users    int
}

// newClones empties root: a clone of a previous run may be cut in the middle and is not trusted.
func newClones(root string, maxBytes, cloneMax int64, timeout, hold time.Duration, now func() time.Time) (*clones, error) {
	if err := os.RemoveAll(root); err != nil {
		return nil, fmt.Errorf("gitadapter: clear cache: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("gitadapter: cache: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &clones{
		root: root, maxBytes: maxBytes, cloneMax: cloneMax, timeout: timeout, hold: hold, now: now,
		ctx: ctx, cancel: cancel, items: map[string]*clone{},
	}, nil
}

// acquire returns a ready clone, held until release. Without one it starts the clone and waits for it at
// most wait; a clone still running answers preparing with the time to come back. A failed clone answers
// its error for hold, then the next call clones again.
func (c *clones) acquire(ctx context.Context, key string, wait, retry time.Duration, start func(ctx context.Context, dir string) error) (*clone, error) {
	c.mu.Lock()
	cl := c.items[key]
	if cl != nil && cl.failed() && c.now().Sub(cl.failedAt) >= c.hold {
		delete(c.items, key)
		cl = nil
	}
	if cl == nil {
		cl = c.start(ctx, key, start)
	}
	done := cl.done
	c.mu.Unlock()

	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-done:
	default:
		return nil, adaptersdk.Preparing(c.now().Add(retry), "partial clone of the repository is running")
	}
	if cl.err != nil {
		return nil, cl.err
	}
	cl.users++
	cl.used = c.now()
	return cl, nil
}

func (c *clones) release(cl *clone) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cl.users--
	cl.used = c.now()
	c.evict(0)
}

// resized records the size of a clone after a fetch changed it.
func (c *clones) resized(cl *clone) {
	size, _ := dirSize(cl.dir)
	c.mu.Lock()
	defer c.mu.Unlock()
	cl.size = size
	c.evict(0)
}

// start runs the clone into a partial directory and moves it into place only when it is complete; the
// caller holds c.mu. The clone watches its size and stops at cloneMax or after timeout.
func (c *clones) start(ctx context.Context, key string, run func(ctx context.Context, dir string) error) *clone {
	cl := &clone{key: key, dir: filepath.Join(c.root, key), done: make(chan struct{})}
	c.items[key] = cl
	c.evict(c.cloneMax)
	log := adaptersdk.Call{}.Log(ctx)
	// The clone outlives the call that started it: it runs on the context of the cache.
	c.wg.Go(func() { //nolint:contextcheck // stopped by Close, not by the call
		tmp := cl.dir + ".partial"
		cctx, stop := context.WithTimeoutCause(log.WithContext(c.ctx), c.timeout, errCloneTimeout)
		defer stop()
		cctx, tooLarge := context.WithCancelCause(cctx)
		watched := make(chan struct{})
		go func() {
			defer close(watched)
			c.watch(cctx, tmp, tooLarge)
		}()
		err := run(cctx, tmp)
		tooLarge(nil)
		<-watched
		// A clone faster than the watch is checked once more when it ends.
		if size, _ := dirSize(tmp); err == nil && size > c.cloneMax {
			err = errCloneTooLarge
		}
		if err == nil {
			err = os.Rename(tmp, cl.dir)
		}
		size, _ := dirSize(cl.dir)
		if err != nil {
			_ = os.RemoveAll(tmp)
			err = cloneError(err)
			log.Warn().Err(err).Msg("clone failed")
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		cl.err, cl.size, cl.used = err, size, c.now()
		if err != nil {
			cl.failedAt = c.now()
		}
		close(cl.done)
		c.evict(0)
	})
	return cl
}

func (c *clones) watch(ctx context.Context, dir string, tooLarge context.CancelCauseFunc) {
	t := time.NewTicker(_sizeEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if size, _ := dirSize(dir); size > c.cloneMax {
				tooLarge(errCloneTooLarge)
				return
			}
		}
	}
}

func cloneError(err error) error {
	switch {
	case errors.Is(err, errCloneTooLarge):
		return adaptersdk.InvalidResponseError("repository exceeds the clone limit of the adapter")
	case errors.Is(err, errCloneTimeout):
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "partial clone did not finish within the time limit")
	}
	if e, ok := errors.AsType[*adaptersdk.Error](err); ok {
		return e
	}
	return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "partial clone failed")
}

// evict removes the least recently used clones nobody holds until the clones and reserve fit maxBytes;
// the caller holds c.mu.
func (c *clones) evict(reserve int64) {
	for {
		var total int64
		var oldest *clone
		for _, cl := range c.items {
			total += cl.size
			if cl.users == 0 && cl.isReady() && (oldest == nil || cl.used.Before(oldest.used)) {
				oldest = cl
			}
		}
		if total+reserve <= c.maxBytes || oldest == nil {
			return
		}
		delete(c.items, oldest.key)
		_ = os.RemoveAll(oldest.dir)
	}
}

// Close stops the clones in progress and waits for them.
func (c *clones) Close() {
	c.cancel()
	c.wg.Wait()
}

// usage is the size of the clones on disk.
func (c *clones) usage() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	var total int64
	for _, cl := range c.items {
		total += cl.size
	}
	return total
}

func (cl *clone) isReady() bool {
	select {
	case <-cl.done:
		return cl.err == nil
	default:
		return false
	}
}

func (cl *clone) failed() bool {
	select {
	case <-cl.done:
		return cl.err != nil
	default:
		return false
	}
}

func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}
