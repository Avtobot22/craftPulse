package github

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_historyCursorPrefix = "history/"
	_historyTokenLimit   = 170
)

type historyCursor struct {
	pull            coordinate
	position        historyPosition
	currentRevision bool
	activityRef     string
}

func parseHistoryCursor(in adaptersdk.FetchInput) (*historyCursor, error) {
	if in.Cursor == nil || !strings.HasPrefix(in.Cursor.NativeId, _historyCursorPrefix) {
		return nil, nil
	}
	bad := adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitHub history cursor")
	parts := strings.SplitN(strings.TrimPrefix(in.Cursor.NativeId, _historyCursorPrefix), "/", 6)
	if len(parts) != 6 || in.SchemaId != adaptersdk.FetchInputSchemaIdMergeRequests || in.Cursor.ModifiedAt.IsZero() {
		return nil, bad
	}
	pullID, ok := numeric(parts[0])
	number, numberErr := strconv.Atoi(parts[1])
	collection, collectionErr := strconv.Atoi(parts[2])
	current, currentErr := strconv.ParseBool(parts[3])
	ref, page := parts[4], parts[5]
	if !ok || numberErr != nil || number <= 0 || collectionErr != nil || collection < 0 ||
		collection > _activityCollection || currentErr != nil || len(page) > _historyTokenLimit {
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
	if !slices.Contains(in.ObjectKeys, adaptersdk.ObjectKey{
		ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: parts[0],
	}) {
		return nil, bad
	}
	return &historyCursor{
		pull:            coordinate{ID: pullID, Number: number, UpdatedAt: in.Cursor.ModifiedAt},
		position:        historyPosition{collection: collection, page: page},
		currentRevision: current,
		activityRef:     ref,
	}, nil
}

func historyNextCursor(p pull, pos historyPosition, currentRevision bool) *envelope.Cursor {
	var ref string
	if pos.collection == _activityCollection && pos.page != "" {
		ref = activityRefKey(p.Head)
	}
	native := _historyCursorPrefix + id(p.ID) + "/" + strconv.Itoa(p.Number) + "/" +
		strconv.Itoa(pos.collection) + "/" + strconv.FormatBool(currentRevision) + "/" + ref + "/" + pos.page
	return &envelope.Cursor{ModifiedAt: p.UpdatedAt, NativeId: native}
}

const _activityRefKeyLength = 24

func activityRefKey(head branch) string {
	// A bounded fingerprint leaves room in the cursor for GitHub's opaque token.
	sum := sha256.Sum256([]byte(id(head.Repo.ID) + "/" + head.Ref))
	return hex.EncodeToString(sum[:_activityRefKeyLength/2])
}
