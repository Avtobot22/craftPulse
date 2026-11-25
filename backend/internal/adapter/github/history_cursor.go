package github

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_historyCursorPrefix = "history/"
	_historyTokenLimit   = 170

	_timelineSeenDraft  = 1 << 0
	_timelineSeenReady  = 1 << 1
	_timelineIncomplete = 1 << 2
	_timelineHasPushes  = 1 << 3
)

type historyCursor struct {
	pull        coordinate
	position    historyPosition
	timeline    timelineProgress
	activityRef string
	profiles    bool
	account     int64
}

func parseHistoryCursor(in adaptersdk.FetchInput) (*historyCursor, error) {
	if in.Cursor == nil || !strings.HasPrefix(in.Cursor.NativeId, _historyCursorPrefix) {
		return nil, nil
	}
	bad := adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitHub history cursor")
	parts := strings.SplitN(strings.TrimPrefix(in.Cursor.NativeId, _historyCursorPrefix), "/", 6)
	if len(parts) != 6 || in.Cursor.ModifiedAt.IsZero() {
		return nil, bad
	}
	pullID, ok := numeric(parts[0])
	collection, collectionErr := strconv.Atoi(parts[2])
	ref, page := parts[4], parts[5]
	state := strings.Split(parts[3], ";")
	legacy := parts[3] == "true" || parts[3] == "false"
	base := 36
	if legacy {
		base = 10
	}
	number, numberErr := strconv.ParseInt(parts[1], base, strconv.IntSize)
	if !ok || numberErr != nil || number <= 0 || collectionErr != nil || collection < 0 ||
		collection > _activityCollection || len(page) > _historyTokenLimit {
		return nil, bad
	}
	if collection == _activityCollection && page != "" {
		if len(ref) != _activityRefKeyLength {
			return nil, bad
		}
	} else if ref != "" {
		return nil, bad
	} else if page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n < 2 {
			return nil, bad
		}
	}
	c := &historyCursor{
		pull:        coordinate{ID: pullID, Number: int(number), UpdatedAt: in.Cursor.ModifiedAt},
		position:    historyPosition{collection: collection, page: page},
		activityRef: ref,
	}
	if !c.parseState(state, legacy) {
		return nil, bad
	}
	if in.SchemaId == adaptersdk.FetchInputSchemaIdAccounts && (!c.profiles || c.position != (historyPosition{})) {
		return nil, bad
	}
	if (!c.profiles || len(in.ObjectKeys) != 0) && !slices.Contains(in.ObjectKeys, adaptersdk.ObjectKey{
		ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: parts[0],
	}) {
		return nil, bad
	}
	return c, nil
}

func (c *historyCursor) parseState(state []string, legacy bool) bool {
	if legacy {
		// Older cursors did not retain timeline state. Rebuild it if the timeline is still pending.
		if c.position.collection == 0 {
			c.position.page = ""
		} else {
			c.timeline.hasPushes = true
		}
		if c.position.collection == _activityCollection {
			// An opaque continuation for the old push-only filter cannot also cover force pushes.
			c.position.page, c.activityRef = "", ""
		}
		return true
	}
	if len(state) < 2 || len(state) > 3 {
		return false
	}
	flags, err := strconv.ParseUint(state[0], 16, 4)
	if err != nil {
		return false
	}
	c.timeline.seenDraft = flags&_timelineSeenDraft != 0
	c.timeline.seenReady = flags&_timelineSeenReady != 0
	c.timeline.incomplete = flags&_timelineIncomplete != 0
	c.timeline.hasPushes = flags&_timelineHasPushes != 0
	if state[1] != "" {
		c.profiles = true
		c.account, err = strconv.ParseInt(state[1], 36, 64)
		if err != nil || c.account < 0 {
			return false
		}
	}
	if len(state) == 3 {
		at, err := time.Parse(time.RFC3339Nano, state[2])
		if err != nil || c.position.collection != 0 || !c.timeline.seenReady {
			return false
		}
		c.timeline.readyAt = &at
	}
	return true
}

func historyNextCursor(p coordinate, pos historyPosition, head branch, timeline timelineProgress, profile *int64) *envelope.Cursor {
	var ref string
	if pos.collection == _activityCollection && pos.page != "" {
		ref = activityRefKey(head)
	}
	var flags uint64
	if timeline.seenDraft {
		flags |= _timelineSeenDraft
	}
	if timeline.seenReady {
		flags |= _timelineSeenReady
	}
	if timeline.incomplete {
		flags |= _timelineIncomplete
	}
	if timeline.hasPushes {
		flags |= _timelineHasPushes
	}
	var account string
	if profile != nil {
		account = strconv.FormatInt(*profile, 36)
	}
	state := strconv.FormatUint(flags, 16) + ";" + account
	if pos.collection == 0 && timeline.readyAt != nil {
		state += ";" + timeline.readyAt.Format(time.RFC3339Nano)
	}
	// Encode the counters compactly to leave room for the opaque activity token.
	native := _historyCursorPrefix + id(p.ID) + "/" + strconv.FormatInt(int64(p.Number), 36) + "/" +
		strconv.Itoa(pos.collection) + "/" + state + "/" + ref + "/" + pos.page
	return &envelope.Cursor{ModifiedAt: p.UpdatedAt, NativeId: native}
}

const _activityRefKeyLength = 24

func activityRefKey(head branch) string {
	// A bounded fingerprint leaves room in the cursor for GitHub's opaque token.
	sum := sha256.Sum256([]byte(id(head.Repo.ID) + "/" + head.Ref))
	return hex.EncodeToString(sum[:_activityRefKeyLength/2])
}
