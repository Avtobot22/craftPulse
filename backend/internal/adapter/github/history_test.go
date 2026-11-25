package github

import (
	"context"
	"math"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func TestHistoryCursorRetainsEarliestReadyTime(t *testing.T) {
	t.Parallel()
	ready := time.Date(2026, 11, 21, 10, 2, 0, 0, time.FixedZone("source", 3*60*60))
	p := pull{coordinate: coordinate{ID: 601, Number: 41, UpdatedAt: ready.Add(time.Hour)}}
	h := history{pull: p, accounts: make(map[int64]account)}
	if err := h.add("timeline", "1", historyItem{Event: "ready_for_review", CreatedAt: &ready}); err != nil {
		t.Fatal(err)
	}
	cursor := historyNextCursor(p.coordinate, historyPosition{page: "13"}, p.Head, h.timeline, nil)
	resume, err := parseHistoryCursor(adaptersdk.FetchInput{
		SchemaId: adaptersdk.FetchInputSchemaIdMergeRequests,
		Cursor:   &adaptersdk.Cursor{ModifiedAt: cursor.ModifiedAt, NativeId: cursor.NativeId},
		ObjectKeys: []adaptersdk.ObjectKey{
			{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"},
		},
	})
	if err != nil || resume == nil {
		t.Fatalf("cannot parse timeline continuation: %v", err)
	}
	if resume.pull.Number != 41 || resume.position.page != "13" {
		t.Fatalf("wrong timeline continuation: %+v", resume)
	}
	h.timeline = resume.timeline
	later := ready.Add(time.Minute)
	if err := h.add("timeline", "2", historyItem{Event: "ready_for_review", CreatedAt: &later}); err != nil {
		t.Fatal(err)
	}
	if h.timeline.readyAt == nil || h.timeline.readyAt.Format(time.RFC3339) != ready.Format(time.RFC3339) {
		t.Fatalf("ready time and offset changed: got %v, want %v", h.timeline.readyAt, ready)
	}
}

func TestProfileCursorFitsLongestActivityContinuation(t *testing.T) {
	t.Parallel()
	p := coordinate{ID: math.MaxInt64, Number: math.MaxInt, UpdatedAt: time.Now()}
	pos := historyPosition{collection: _activityCollection, page: strings.Repeat("/", _historyTokenLimit)}
	accountID := int64(math.MaxInt64)
	cursor := historyNextCursor(p, pos, branch{}, timelineProgress{hasPushes: true}, &accountID)
	if len(cursor.NativeId) > 255 {
		t.Fatalf("cursor exceeds the 255-byte limit: %d", len(cursor.NativeId))
	}
	resume, err := parseHistoryCursor(adaptersdk.FetchInput{
		SchemaId: adaptersdk.FetchInputSchemaIdMergeRequests,
		Cursor:   &adaptersdk.Cursor{ModifiedAt: cursor.ModifiedAt, NativeId: cursor.NativeId},
		ObjectKeys: []adaptersdk.ObjectKey{
			{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: id(p.ID)},
		},
	})
	if err != nil || resume == nil {
		t.Fatalf("cannot parse profile continuation: %v", err)
	}
	if !resume.profiles || !resume.timeline.hasPushes || resume.account != accountID || resume.position != pos {
		t.Fatalf("wrong profile continuation: %+v", resume)
	}
}

func TestLegacyActivityCursorRestartsWithExpandedFilter(t *testing.T) {
	t.Parallel()
	resume, err := parseHistoryCursor(adaptersdk.FetchInput{
		SchemaId: adaptersdk.FetchInputSchemaIdMergeRequests,
		Cursor: &adaptersdk.Cursor{
			ModifiedAt: time.Now(),
			NativeId:   "history/601/1/4/false/" + activityRefKey(branch{}) + "/old-token",
		},
		ObjectKeys: []adaptersdk.ObjectKey{
			{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "601"},
		},
	})
	if err != nil || resume == nil {
		t.Fatalf("cannot parse legacy continuation: %v", err)
	}
	if resume.position != (historyPosition{collection: _activityCollection}) || resume.activityRef != "" {
		t.Fatalf("legacy continuation should restart activity: %+v", resume)
	}
}

func TestEnrichProfilesResumesAfterSoftDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ids := make([]int64, 100)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		var enriched []int64
		visit := func(ctx context.Context, n int64) error {
			timer := time.NewTimer(300 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "fixture timed out")
			case <-timer.C:
				enriched = append(enriched, n)
				return nil
			}
		}
		var after int64
		for step := 0; ; step++ {
			if step >= 5 {
				t.Fatal("did not reach the end of the profile list")
			}
			ctx, cancel := context.WithTimeout(t.Context(), adaptersdk.SourceTimeout)
			start := time.Now()
			last, pending, err := enrichProfiles(ctx, ids, make(map[int64]bool), after, step != 0, visit)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			if elapsed := time.Since(start); elapsed >= adaptersdk.SourceTimeout {
				t.Fatalf("profile batch used the whole Fetch budget: %s", elapsed)
			}
			if step == 0 && (!pending || last != 26) {
				t.Fatalf("expected to pause at the soft deadline after account 26, got %d, pending %t", last, pending)
			}
			if !pending {
				break
			}
			if last <= after {
				t.Fatalf("profile cursor did not advance: was %d, got %d", after, last)
			}
			after = last
		}
		if !slices.Equal(ids, enriched) {
			t.Fatalf("unexpected accounts after resuming enrichment: %v", enriched)
		}
	})
}
