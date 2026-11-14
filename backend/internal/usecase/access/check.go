package access

import (
	"context"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

const (
	// _cacheSize bounds the decisions of one process: about 100 bytes each, a few megabytes in all.
	_cacheSize = 50_000
	// _checkTimeout keeps a read within the 5 s of NFR-001 (TZ 10.3).
	_checkTimeout = 2 * time.Second
	// _maxAge is the oldest decision of a source a read may rely on (NFR-011).
	_maxAge = 60 * time.Second
	// _parallelChecks bounds the checks one read runs at once.
	_parallelChecks = 8
)

type checkKey struct {
	org, user, item entity.ID
}

func (k checkKey) String() string {
	return k.org.String() + "/" + k.user.String() + "/" + k.item.String()
}

type verdict struct {
	decision string
	expires  time.Time
}

// target is an area or an object with its own restriction, checked in the connection of its area with the
// account of the user in that instance.
type target struct {
	id      entity.ID
	source  entity.AreaSource
	account entity.UserAccount
	object  *entity.RestrictedObject
}

func (t target) item() entity.SourceAccessItem {
	if t.object != nil {
		return entity.SourceAccessItem{ObjectKind: string(t.object.Kind), NativeNS: t.object.NativeNS, NativeID: t.object.NativeID}
	}
	return entity.SourceAccessItem{AreaKind: t.source.Kind, NativeID: t.source.NativeScopeID}
}

// checks keep the answers of CheckAccess in memory of the process by the pair of a user and an area or an
// object, for at most 60 s and at most the freshness the adapter names (TZ 12.5.2). Simultaneous misses of
// one pair wait for one check. The library TTL is the hard bound in real time; the clock of the module
// decides the age, so the bound holds in tests with a manual clock as well.
type checks struct {
	sources Sources
	metrics Metrics
	clock   clock.Clock
	timeout time.Duration
	cache   *expirable.LRU[checkKey, verdict]
	flight  singleflight.Group
}

func newChecks(s Sources, m Metrics, c clock.Clock, size int, timeout time.Duration) *checks {
	return &checks{sources: s, metrics: m, clock: c, timeout: timeout, cache: expirable.NewLRU[checkKey, verdict](size, nil, _maxAge)}
}

// decide returns allowed, denied or unknown; a failed or late check is unknown and is not kept, so the next
// read checks again.
func (c *checks) decide(ctx context.Context, org, user entity.ID, t target) string {
	key := checkKey{org: org, user: user, item: t.id}
	if v, ok := c.cache.Get(key); ok && c.clock.Now().Before(v.expires) {
		c.metrics.CacheLookup(true)
		return v.decision
	}
	c.metrics.CacheLookup(false)
	v, _, _ := c.flight.Do(key.String(), func() (any, error) {
		return c.check(ctx, key, t), nil
	})
	decision, ok := v.(string)
	if !ok {
		return entity.SourceUnknown
	}
	return decision
}

// check runs without the cancellation of the caller: the readers that wait for it in flight need its answer
// even when the first reader went away.
func (c *checks) check(ctx context.Context, key checkKey, t target) string {
	start := c.clock.Now()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
	defer cancel()
	res, err := c.sources.CheckAccess(ctx, key.org, t.source.ConnectionID, entity.SourceAccessInput{
		UserID: key.user, AccountNativeID: t.account.NativeID, Items: []entity.SourceAccessItem{t.item()},
	})
	if err != nil || len(res.Decisions) != 1 {
		c.metrics.CheckFailed(t.source.ConnectionID)
		return entity.SourceUnknown
	}
	decision := res.Decisions[0].Decision
	ttl := _maxAge
	if res.MaxAgeSeconds != nil {
		ttl = min(ttl, time.Duration(*res.MaxAgeSeconds)*time.Second)
	}
	if ttl > 0 {
		c.cache.Add(key, verdict{decision: decision, expires: start.Add(ttl)})
	}
	return decision
}

type noMetrics struct{}

func (noMetrics) CacheLookup(bool) {}

func (noMetrics) CheckFailed(entity.ID) {}
