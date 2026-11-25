package normalize

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type mrDiffCandidate struct {
	mr   entity.SourceKey
	fact entity.MRDiffFact
}

func (in *input) mrDiff(rec *envelope.Record, base baseFact) (mrDiffCandidate, error) {
	var p envelope.MrDiffV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return mrDiffCandidate{}, fmt.Errorf("normalize: decode MR diff: %w", err)
	}
	files := make([]entity.MRDiffFile, len(p.Files))
	for i := range p.Files {
		f := &p.Files[i]
		ranges := make([]entity.MRDiffRange, len(f.HeadRanges))
		for j, h := range f.HeadRanges {
			ranges[j] = entity.MRDiffRange(h)
		}
		files[i] = entity.MRDiffFile{
			OldPath: f.OldPath, NewPath: f.NewPath, NewFile: f.NewFile, DeletedFile: f.DeletedFile, RenamedFile: f.RenamedFile,
			Binary: f.Binary, AddedLines: f.AddedLines, DeletedLines: f.DeletedLines, HeadRanges: ranges,
		}
	}
	return mrDiffCandidate{
		mr: in.mrKey(p.MrNativeId),
		fact: entity.MRDiffFact{
			MRDiff: entity.MRDiff{RevisionKey: rec.NativeId, HeadSHA: p.HeadSha, BaseSHA: p.BaseSha, Completeness: string(p.Completeness), Files: files},
			Record: base.ref, ContentHash: base.revision.Hash,
		},
	}, nil
}

func (w *writer) mergeRequestDiffs(ctx context.Context, in *input) error {
	if len(in.mrDiffs) == 0 {
		return nil
	}
	area := w.objects[in.area].ID
	facts := make([]entity.MRDiffFact, len(in.mrDiffs))
	for i := range in.mrDiffs {
		diff := &in.mrDiffs[i]
		facts[i] = diff.fact
		facts[i].MRID = w.objects[diff.mr].ID
	}
	count, err := w.u.d.Repo.InsertMRDiffs(ctx, w.org, area, in.receipt, w.seq, facts)
	if err != nil {
		return err
	}
	if count > 0 {
		w.changes.add(area, "mr_diff", nil)
	}
	return nil
}

// MRDiffAt finds the latest revision for head on the knowledge cut and reads its best available diff.
func (u *UseCase) MRDiffAt(ctx context.Context, scope entity.Scope, org, mr entity.ID, head string, seq int64) (entity.MRDiff, error) {
	if len(scope.Areas()) == 0 {
		return entity.MRDiff{}, entity.ErrNotFound
	}
	var out entity.MRDiff
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadMRDiff(ctx, org, mr, scope.Areas(), scope.Objects(), head, seq)
		return err
	})
	return out, err
}

// MRChangedLinesAt returns line ranges in the head file. Removed files are available through MRDiffAt.
func (u *UseCase) MRChangedLinesAt(ctx context.Context, scope entity.Scope, org, mr entity.ID, head, file string, seq int64) (entity.MRChangedLines, error) {
	diff, err := u.MRDiffAt(ctx, scope, org, mr, head, seq)
	if err != nil {
		return entity.MRChangedLines{}, err
	}
	i := slices.IndexFunc(diff.Files, func(f entity.MRDiffFile) bool { return !f.DeletedFile && f.NewPath == file })
	if i < 0 {
		return entity.MRChangedLines{}, entity.ErrNotFound
	}
	return entity.MRChangedLines{
		File: diff.Files[i], Completeness: diff.Completeness, RevisionID: diff.RevisionID, BaseSHA: diff.BaseSHA, HeadSHA: diff.HeadSHA,
	}, nil
}

// SummaryDiff reads the diff of the exact head used by the summary worker.
func (u *UseCase) SummaryDiff(ctx context.Context, org entity.ID, mr entity.SummaryMR) (entity.MRDiff, error) {
	var out entity.MRDiff
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadMRDiff(ctx, org, mr.MRID, []entity.ID{mr.AreaID}, nil, mr.HeadSHA, mr.KnowledgeSeq)
		return err
	})
	return out, err
}
