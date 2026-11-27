package normalize

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type analysisCandidate struct {
	run       entity.AnalysisRun
	component *envelope.ScanComponent
}

func analysisCandidates(pkg *envelope.Package, recordCount int) ([]analysisCandidate, error) {
	if pkg.ScanManifest == nil {
		return nil, nil
	}
	numbers := make(map[string]int)
	for no := 1; no <= recordCount; no++ {
		rec, ok, err := pkg.Record(no)
		if err != nil {
			return nil, err
		}
		if ok && rec.RecordKind == _reportFileRecord && rec.File != nil {
			numbers[rec.File.Path] = no
		}
	}
	var out []analysisCandidate
	for i := range pkg.ScanManifest.Tools {
		t := &pkg.ScanManifest.Tools[i]
		if t.Sha == nil || string(t.ReportKind) == entity.AnalysisSonar {
			continue
		}
		r := entity.AnalysisRun{
			AnalysisScope: entity.AnalysisScope{SHA: *t.Sha},
			ToolID:        t.Id, RecordNos: make([]int, len(t.Reports)),
			AnalysisRunMetadata: analysisMetadata(t),
		}
		for j, file := range t.Reports {
			r.RecordNos[j] = numbers[file.File.Path]
		}
		out = append(out, analysisCandidate{run: r, component: t.Component})
	}
	return out, nil
}

func analysisMetadata(t *envelope.ScanToolRun) entity.AnalysisRunMetadata {
	m := entity.AnalysisRunMetadata{
		Kind: string(t.ReportKind), Producer: t.Producer, Profile: deref(t.ProfileId), Mode: string(t.Mode),
		Language: deref(t.Language), ScopeHash: t.ScopeHash,
		QueueSeconds: t.QueueSeconds, ExitClass: string(t.ExitClass), Completeness: string(t.Completeness),
		ScannedFiles: t.ScannedCount, EligibleFiles: t.EligibleFileCount,
		DBVersion: deref(t.DbVersion), DBPublished: t.DbPublishedAt,
	}
	if !t.StartedAt.IsZero() {
		m.StartedAt = &t.StartedAt
	}
	if !t.FinishedAt.IsZero() {
		m.FinishedAt = &t.FinishedAt
	}
	if t.Component != nil {
		m.PathPrefix = t.Component.PathPrefix
	}
	if h := t.History; h != nil {
		m.History = &entity.ScanHistory{FromSHA: h.FromSha, ToSHA: h.ToSha, Truncated: h.Truncated, FromAt: h.FromAt, ToAt: h.ToAt}
	}
	// These builtin profiles already published scanned and skipped eligible files before the explicit total was added.
	if m.EligibleFiles == nil && m.Mode == entity.CheckModeBuiltin &&
		(m.Kind == entity.AnalysisCode || m.Kind == entity.AnalysisSAST && m.Producer == "opengrep") &&
		t.ScannedCount != nil && t.SkippedCount != nil {
		m.EligibleFiles = new(*t.ScannedCount + *t.SkippedCount)
	}
	return m
}

func (w *writer) analysisRuns(ctx context.Context, in *input) error {
	if len(in.analysis) == 0 {
		return nil
	}
	area := w.objects[in.area].ID
	var components []entity.Component
	if slices.ContainsFunc(in.analysis, func(c analysisCandidate) bool { return c.component != nil }) {
		var err error
		components, err = w.u.d.Accounts.RepositoryComponents(ctx, w.org, area)
		if err != nil {
			return err
		}
	}
	runs := make([]entity.AnalysisRun, 0, len(in.analysis))
	for i := range in.analysis {
		c := &in.analysis[i]
		r := c.run
		r.ReceiptID, r.RepositoryID, r.KnownSeq = in.receipt, area, w.seq
		if component := componentOf(components, c.component); component != nil {
			r.ComponentID = *component
		}
		runs = append(runs, r)
	}
	return w.u.d.Repo.InsertAnalysisRuns(ctx, w.org, runs)
}

func (u *UseCase) AnalysisFacts(ctx context.Context, org entity.ID, scope entity.AnalysisScope, seq int64, asOf time.Time) (entity.AnalysisFacts, error) {
	var out entity.AnalysisFacts
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReadAnalysisFacts(ctx, org, scope, seq, asOf)
		return err
	})
	return out, err
}

func (u *UseCase) ReceiptAnalysisScopes(ctx context.Context, org, receipt entity.ID) ([]entity.AnalysisScope, error) {
	var out []entity.AnalysisScope
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReceiptAnalysisScopes(ctx, org, receipt)
		return err
	})
	return out, err
}
