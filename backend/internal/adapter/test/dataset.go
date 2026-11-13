package testadapter

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

//go:embed dataset.json
var _datasetJSON []byte

// dataset is a recorded Jira DC project: each feed lists what the source returned in the order of the
// increment cursor, so an object changed while it was read appears again with its later version.
type dataset struct {
	InstanceKey string                 `json:"instance_key"`
	Version     string                 `json:"version"`
	RecordedAt  time.Time              `json:"recorded_at"`
	Projects    []project              `json:"projects"`
	Feeds       map[string][]entry     `json:"feeds"`
	Truncated   []truncatedHistoryJSON `json:"truncated"`
}

type project struct {
	NativeID string `json:"native_id"`
	Name     string `json:"name"`
}

type entry struct {
	NativeNS   string          `json:"native_ns"`
	NativeID   string          `json:"native_id"`
	Revision   string          `json:"revision"`
	ModifiedAt time.Time       `json:"modified_at"`
	Payload    json.RawMessage `json:"payload"`
}

type truncatedHistoryJSON struct {
	RecordKind  string    `json:"record_kind"`
	NativeNS    string    `json:"native_ns"`
	NativeID    string    `json:"native_id"`
	HistoryFrom time.Time `json:"history_from"`
}

// _feedKinds are the record kind, class and payload schema of each data kind the test adapter serves.
var _feedKinds = map[string]struct{ kind, class, schema string }{
	"tasks":       {kind: "task", class: "object", schema: "task/1"},
	"task_events": {kind: "task_event", class: "event", schema: "task_event/1"},
}

func loadDataset() (*dataset, error) {
	var d dataset
	if err := json.Unmarshal(_datasetJSON, &d); err != nil {
		return nil, fmt.Errorf("testadapter: dataset: %w", err)
	}
	for id, feed := range d.Feeds {
		if _, ok := _feedKinds[id]; !ok {
			return nil, fmt.Errorf("testadapter: dataset: feed %s is not served", id)
		}
		slices.SortStableFunc(feed, func(a, b entry) int {
			return cmp.Or(a.ModifiedAt.Compare(b.ModifiedAt), cmp.Compare(a.NativeID, b.NativeID))
		})
	}
	return &d, nil
}

func (d *dataset) hasProject(id string) (project, bool) {
	for _, p := range d.Projects {
		if p.NativeID == id {
			return p, true
		}
	}
	return project{}, false
}

// after is the position of the first entry past the cursor.
func after(feed []entry, modifiedAt time.Time, nativeID string) int {
	i, _ := slices.BinarySearchFunc(feed, entry{ModifiedAt: modifiedAt, NativeID: nativeID}, func(e, target entry) int {
		return cmp.Or(e.ModifiedAt.Compare(target.ModifiedAt), cmp.Compare(e.NativeID, target.NativeID))
	})
	for i < len(feed) && feed[i].ModifiedAt.Equal(modifiedAt) && feed[i].NativeID == nativeID {
		i++
	}
	return i
}

// inPeriod keeps the entries changed in [from, to); nil bounds are open.
func inPeriod(feed []entry, from, to *time.Time) []entry {
	out := make([]entry, 0, len(feed))
	for _, e := range feed {
		if (from == nil || !e.ModifiedAt.Before(*from)) && (to == nil || e.ModifiedAt.Before(*to)) {
			out = append(out, e)
		}
	}
	return out
}

// latest keeps the last version of every object, in the order of those last versions.
func latest(feed []entry) []entry {
	last := make(map[[2]string]int, len(feed))
	for i, e := range feed {
		last[[2]string{e.NativeNS, e.NativeID}] = i
	}
	out := make([]entry, 0, len(last))
	for i, e := range feed {
		if last[[2]string{e.NativeNS, e.NativeID}] == i {
			out = append(out, e)
		}
	}
	return out
}
