package normalize

import (
	"bytes"
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_factCIRun     = "ci_run"
	_factCIAttempt = "ci_attempt"
)

func (w *writer) writeCIFacts(ctx context.Context, in *input) error {
	if err := w.ciRuns(ctx, in); err != nil {
		return err
	}
	return w.ciAttempts(ctx, in.ciAttempts)
}

func (w *writer) ciRuns(ctx context.Context, in *input) error {
	candidates := in.ciRuns
	if len(candidates) == 0 {
		return nil
	}
	ids := make([]entity.ID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, w.objects[c.key].ID)
	}
	current, err := w.u.d.Repo.CurrentCIRunVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	byKey := make(map[entity.ID]entity.CIRunVersion, len(current))
	for i := range current {
		byKey[current[i].RunID] = current[i]
	}
	revisions, err := w.u.d.Repo.KnownCIRunVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	mrs, err := w.runMRs(ctx, in)
	if err != nil {
		return err
	}
	var (
		supersede  []entity.ID
		insert     []entity.CIRunVersion
		placements []entity.ObjectPlacement
	)
	for i, c := range candidates {
		obj := w.objects[c.key]
		if obj.DeletedAt != nil || !w.admits(revisions[obj.ID], c.version.Revision, c.version.Record.RecordNo) {
			continue
		}
		cur, has := byKey[obj.ID]
		if has && (olderTimes(c.version.SourceUpdatedAt, c.version.ObservedAt, cur.SourceUpdatedAt, cur.ObservedAt, false) || bytes.Equal(c.version.Revision.Hash, cur.Revision.Hash)) {
			continue
		}
		v := c.version
		v.RunID, v.AreaID, v.KnownSeq = obj.ID, w.objects[c.area].ID, w.seq
		if mr, ok := mrs[i]; ok {
			v.MRID = &mr
		}
		if c.parent != nil {
			parent := w.objects[*c.parent].ID
			v.ParentRunID = &parent
		}
		if obj.CurrentAreaID == nil || *obj.CurrentAreaID != v.AreaID {
			placements = append(placements, entity.ObjectPlacement{ObjectID: obj.ID, AreaID: v.AreaID})
			area := v.AreaID
			obj.CurrentAreaID = &area
			w.objects[c.key] = obj
		}
		w.changes.add(v.AreaID, _factCIRun, changedAt(v.StartedAt, v.FinishedAt))
		if has {
			w.changes.add(cur.AreaID, _factCIRun, changedAt(cur.StartedAt, cur.FinishedAt))
			supersede = append(supersede, cur.ID)
		}
		insert = append(insert, v)
	}
	if err := w.u.d.Repo.SupersedeCIRunVersions(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	if err := w.u.d.Repo.InsertCIRunVersions(ctx, w.org, insert); err != nil {
		return err
	}
	if err := w.u.d.Repo.PlaceObjects(ctx, w.org, placements); err != nil {
		return err
	}
	for i := range insert {
		v := &insert[i]
		if v.MRID != nil {
			if _, err := w.u.d.Links.Record(ctx, w.org, entity.LinkProposal{
				Type: entity.LinkMRRun, From: *v.MRID, To: v.RunID,
				Method: entity.LinkSourceExplicit, Record: &v.Record,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// runMRs are the MRs of the runs by their index: a run belongs to the MR the source names only when the MR
// is in the repository of the run and the SHA of the run is a head of the MR (TZ mvp-scope, rule 4).
func (w *writer) runMRs(ctx context.Context, in *input) (map[int]entity.ID, error) {
	var (
		matches []entity.RunMRMatch
		index   []int
	)
	for i, c := range in.ciRuns {
		if c.mr == "" {
			continue
		}
		matches = append(matches, entity.RunMRMatch{MRNativeID: c.mr, AreaID: w.objects[c.area].ID, SHA: c.version.SHA})
		index = append(index, i)
	}
	found, err := w.u.d.Repo.MatchRunMRs(ctx, w.org, in.instance, matches)
	if err != nil {
		return nil, err
	}
	out := make(map[int]entity.ID, len(found))
	for i, mr := range found {
		out[index[i]] = mr
	}
	return out, nil
}

func (w *writer) ciAttempts(ctx context.Context, candidates []*ciAttemptCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	ids := make([]entity.ID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, w.objects[c.key].ID)
	}
	current, err := w.u.d.Repo.CurrentCIAttemptVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	byKey := make(map[entity.ID]entity.CIAttemptVersion, len(current))
	for i := range current {
		byKey[current[i].AttemptID] = current[i]
	}
	revisions, err := w.u.d.Repo.KnownCIAttemptVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	var (
		supersede []entity.ID
		insert    []entity.CIAttemptVersion
	)
	for _, c := range candidates {
		obj, run := w.objects[c.key], w.objects[c.run]
		if obj.DeletedAt != nil || run.DeletedAt != nil || !w.admits(revisions[obj.ID], c.version.Revision, c.version.Record.RecordNo) {
			continue
		}
		cur, has := byKey[obj.ID]
		if has && (olderAttempt(c.version, cur) || bytes.Equal(c.version.Revision.Hash, cur.Revision.Hash)) {
			continue
		}
		v := c.version
		v.AttemptID, v.RunID, v.KnownSeq = obj.ID, run.ID, w.seq
		if run.CurrentAreaID != nil {
			w.changes.add(*run.CurrentAreaID, _factCIAttempt, changedAt(v.StartedAt, v.FinishedAt))
			if has {
				w.changes.add(*run.CurrentAreaID, _factCIAttempt, changedAt(cur.StartedAt, cur.FinishedAt))
			}
		}
		if has {
			supersede = append(supersede, cur.ID)
		}
		insert = append(insert, v)
	}
	if err := w.u.d.Repo.SupersedeCIAttemptVersions(ctx, w.org, supersede, w.seq, w.now); err != nil {
		return err
	}
	return w.u.d.Repo.InsertCIAttemptVersions(ctx, w.org, insert)
}
