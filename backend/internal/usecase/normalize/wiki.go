package normalize

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var _wikiPageID = regexp.MustCompile(`^\d{1,255}$`)

type wikiCandidate struct {
	key  entity.SourceKey
	page entity.WikiPage
}

func (in *input) wikiDocument(rec *envelope.Record, instanceKey string, base baseFact, observedAt time.Time) (*wikiCandidate, error) {
	var d envelope.DocumentV1
	if err := json.Unmarshal(rec.Payload, &d); err != nil {
		return nil, fmt.Errorf("normalize: decode wiki document: %w", err)
	}
	if d.DocFormat != envelope.DocumentV1DocFormatConfluence {
		return nil, nil
	}
	version, err := strconv.ParseInt(d.VersionRef, 10, 64)
	if err != nil || version < 1 || strconv.FormatInt(version, 10) != d.VersionRef ||
		in.area.Kind != entity.ObjectKind(entity.ScopeWikiSpace) || d.ContainerNativeId != in.area.NativeID ||
		rec.NativeNs != "" || !_wikiPageID.MatchString(rec.NativeId) || (d.PageNativeId != nil && *d.PageNativeId != rec.NativeId) ||
		!d.AclState.Valid() {
		return nil, errors.New("normalize: invalid wiki document identity, version or ACL")
	}
	ancestors := []string{}
	if d.AncestorNativeIds != nil {
		ancestors = *d.AncestorNativeIds
	}
	if slices.Contains(ancestors, rec.NativeId) {
		return nil, errors.New("normalize: wiki page is its own ancestor")
	}
	var hash []byte
	if d.ContentSha256 != nil {
		hash, err = hex.DecodeString(*d.ContentSha256)
		if err != nil || len(hash) != 32 {
			return nil, errors.New("normalize: invalid wiki document hash")
		}
	}
	sourceURL := d.SourceUrl
	if sourceURL == nil && validWikiURL(instanceKey) {
		sourceURL = new(strings.TrimRight(instanceKey, "/") + "/pages/viewpage.action?pageId=" + rec.NativeId)
	}
	if sourceURL != nil && !validWikiURL(*sourceURL) {
		return nil, errors.New("normalize: invalid wiki document source URL")
	}
	at := base.observed
	if d.UpdatedAt != nil {
		at = *d.UpdatedAt
	}
	return &wikiCandidate{
		key: entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectDocument, NativeID: rec.NativeId},
		page: entity.WikiPage{
			PageNativeID: rec.NativeId, VersionRef: d.VersionRef, Title: d.Title,
			ContentHash: hash, ACLState: string(d.AclState), Restriction: d.Restriction, AncestorIDs: ancestors,
			SourceURL: sourceURL, UpdatedAt: at, ObservedAt: observedAt, Deleted: d.Deleted != nil && *d.Deleted,
		},
	}, nil
}

func validWikiURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func (w *writer) wikiDocuments(ctx context.Context, in *input) error {
	area := w.objects[in.area].ID
	for _, c := range in.wikiPages {
		c.page.DocumentID = w.objects[c.key].ID
		changed, err := w.u.d.Repo.SaveWikiPage(ctx, w.org, area, w.seq, c.page)
		if err != nil {
			return err
		}
		if changed {
			w.changes.add(area, "documents", &c.page.UpdatedAt)
		}
	}
	return nil
}
