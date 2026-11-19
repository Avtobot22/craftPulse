package normalize

import (
	"bytes"
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (w *writer) writeMRFacts(ctx context.Context, in *input) error {
	if err := w.repositories(ctx, in.repositories); err != nil {
		return err
	}
	if err := w.mergeRequests(ctx, in.mrs); err != nil {
		return err
	}
	if err := w.mergeRequestRevisions(ctx, in.mrRevisions); err != nil {
		return err
	}
	return w.mergeRequestEvents(ctx, in.mrEvents)
}

func (w *writer) repositories(ctx context.Context, candidates []*repositoryCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	ids := make([]entity.ID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, w.objects[c.key].ID)
	}
	current, err := w.u.d.Repo.CurrentRepositoryVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	byKey := make(map[entity.ID]entity.RepositoryVersion, len(current))
	for i := range current {
		v := &current[i]
		byKey[v.RepositoryID] = *v
	}
	revisions, err := w.u.d.Repo.KnownRepositoryVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	var supersede []entity.ID
	var insert []entity.RepositoryVersion
	for _, c := range candidates {
		obj := w.objects[c.key]
		key := obj.ID
		if obj.DeletedAt != nil || !w.admits(revisions[key], c.version.Revision, c.version.Record.RecordNo) {
			continue
		}
		cur, has := byKey[key]
		if has && (olderTimes(c.version.SourceUpdatedAt, c.version.ObservedAt, cur.SourceUpdatedAt, cur.ObservedAt, c.snapshot) || bytes.Equal(c.version.Revision.Hash, cur.Revision.Hash)) {
			continue
		}
		v := c.version
		v.RepositoryID, v.KnownSeq = obj.ID, w.seq
		w.changes.add(obj.ID, "repository", v.SourceUpdatedAt)
		if has {
			supersede = append(supersede, cur.ID)
		}
		insert = append(insert, v)
	}
	if err := w.u.d.Repo.SupersedeRepositoryVersions(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	return w.u.d.Repo.InsertRepositoryVersions(ctx, w.org, insert)
}

func (w *writer) mergeRequests(ctx context.Context, candidates []*mrCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	ids := make([]entity.ID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, w.objects[c.key].ID)
	}
	current, err := w.u.d.Repo.CurrentMRVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	byKey := make(map[entity.ID]entity.MRVersion, len(current))
	for i := range current {
		v := &current[i]
		byKey[v.MRID] = *v
	}
	revisions, err := w.u.d.Repo.KnownMRVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	var supersede []entity.ID
	var insert []entity.MRVersion
	var placements []entity.ObjectPlacement
	for _, c := range candidates {
		obj := w.objects[c.key]
		key := obj.ID
		if obj.DeletedAt != nil || !w.admits(revisions[key], c.version.Revision, c.version.Record.RecordNo) {
			continue
		}
		cur, has := byKey[key]
		if has && (olderTimes(c.version.SourceUpdatedAt, c.version.ObservedAt, cur.SourceUpdatedAt, cur.ObservedAt, c.snapshot) || bytes.Equal(c.version.Revision.Hash, cur.Revision.Hash)) {
			continue
		}
		v := c.version
		v.MRID, v.KnownSeq = obj.ID, w.seq
		v.AreaID = w.objects[c.area].ID
		v.AuthorAccountID = w.account(c.author)
		for _, native := range c.reviewers {
			if id := w.account(native); id != nil && !slices.Contains(v.ReviewerAccountIDs, *id) {
				v.ReviewerAccountIDs = append(v.ReviewerAccountIDs, *id)
			}
		}
		if obj.CurrentAreaID == nil || *obj.CurrentAreaID != v.AreaID {
			placements = append(placements, entity.ObjectPlacement{ObjectID: obj.ID, AreaID: v.AreaID})
			area := v.AreaID
			obj.CurrentAreaID = &area
			w.objects[c.key] = obj
		}
		changed := v.SourceUpdatedAt
		if changed == nil {
			changed = &v.OpenedAt
		}
		w.changes.add(v.AreaID, "merge_request", changed)
		if has && cur.AreaID != v.AreaID {
			w.changes.add(cur.AreaID, "merge_request", changed)
		}
		if has {
			supersede = append(supersede, cur.ID)
		}
		insert = append(insert, v)
	}
	if err := w.u.d.Repo.SupersedeMRVersions(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	if err := w.u.d.Repo.InsertMRVersions(ctx, w.org, insert); err != nil {
		return err
	}
	return w.u.d.Repo.PlaceObjects(ctx, w.org, placements)
}

func (w *writer) mergeRequestEvents(ctx context.Context, candidates []*mrEventCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	keys := make([]entity.MRFactKey, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, entity.MRFactKey{MRID: w.objects[c.mr].ID, Key: c.event.EventKey})
	}
	current, err := w.u.d.Repo.CurrentMREvents(ctx, w.org, keys)
	if err != nil {
		return err
	}
	byKey := make(map[entity.MRFactKey]entity.MREvent, len(current))
	for i := range current {
		v := &current[i]
		byKey[entity.MRFactKey{MRID: v.MRID, Key: v.EventKey}] = *v
	}
	revisions, err := w.u.d.Repo.KnownMREvents(ctx, w.org, keys)
	if err != nil {
		return err
	}
	var supersede []entity.ID
	var insert []entity.MREvent
	for _, c := range candidates {
		obj := w.objects[c.mr]
		key := entity.MRFactKey{MRID: obj.ID, Key: c.event.EventKey}
		if obj.DeletedAt != nil || !w.admits(revisions[key], c.event.Revision, c.event.Record.RecordNo) {
			continue
		}
		cur, has := byKey[key]
		if has && bytes.Equal(c.event.Revision.Hash, cur.Revision.Hash) {
			continue
		}
		v := c.event
		v.MRID, v.KnownSeq = obj.ID, w.seq
		v.ActorAccountID, v.TargetAccountID = w.account(c.actor), w.account(c.target)
		if obj.CurrentAreaID != nil {
			w.changes.add(*obj.CurrentAreaID, "mr_event", &v.EventTime)
			if has {
				w.changes.add(*obj.CurrentAreaID, "mr_event", &cur.EventTime)
			}
		}
		if has {
			supersede = append(supersede, cur.ID)
		}
		insert = append(insert, v)
	}
	if err := w.u.d.Repo.SupersedeMREvents(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	return w.u.d.Repo.InsertMREvents(ctx, w.org, insert)
}

func (w *writer) mergeRequestRevisions(ctx context.Context, candidates []*mrRevisionCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	keys := make([]entity.MRFactKey, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, entity.MRFactKey{MRID: w.objects[c.mr].ID, Key: c.revision.RevisionKey})
	}
	current, err := w.u.d.Repo.CurrentMRRevisions(ctx, w.org, keys)
	if err != nil {
		return err
	}
	byKey := make(map[entity.MRFactKey]entity.MRRevision, len(current))
	for i := range current {
		v := &current[i]
		byKey[entity.MRFactKey{MRID: v.MRID, Key: v.RevisionKey}] = *v
	}
	revisions, err := w.u.d.Repo.KnownMRRevisions(ctx, w.org, keys)
	if err != nil {
		return err
	}
	var supersede []entity.ID
	var insert []entity.MRRevision
	for _, c := range candidates {
		obj := w.objects[c.mr]
		key := entity.MRFactKey{MRID: obj.ID, Key: c.revision.RevisionKey}
		if obj.DeletedAt != nil || !w.admits(revisions[key], c.revision.Revision, c.revision.Record.RecordNo) {
			continue
		}
		cur, has := byKey[key]
		if has && bytes.Equal(c.revision.Revision.Hash, cur.Revision.Hash) {
			continue
		}
		v := c.revision
		v.MRID, v.KnownSeq = obj.ID, w.seq
		if obj.CurrentAreaID != nil {
			w.changes.add(*obj.CurrentAreaID, "mr_revision", &v.CreatedAt)
			if has {
				w.changes.add(*obj.CurrentAreaID, "mr_revision", &cur.CreatedAt)
			}
		}
		if has {
			supersede = append(supersede, cur.ID)
		}
		insert = append(insert, v)
	}
	if err := w.u.d.Repo.SupersedeMRRevisions(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	return w.u.d.Repo.InsertMRRevisions(ctx, w.org, insert)
}
