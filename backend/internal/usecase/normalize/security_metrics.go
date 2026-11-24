package normalize

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// SecurityFacts reads the available reports and findings at the scope's knowledge number.
func (u *UseCase) SecurityFacts(ctx context.Context, org entity.ID, scope entity.SecurityScope) (entity.SecurityFacts, error) {
	out := entity.SecurityFacts{Scope: scope}
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out.Reports, err = u.readSecurityReports(ctx, org, scope)
		if err != nil {
			return err
		}
		ids := make([]entity.ID, 0, len(out.Reports))
		for i := range out.Reports {
			r := &out.Reports[i]
			if r.RejectedReason == "" && r.ExitClass != "failed" && r.ExitClass != "timeout" {
				ids = append(ids, r.ID)
			}
		}
		if len(ids) != 0 {
			out.Findings, err = u.d.Repo.ReadSecurityFindings(ctx, org, ids)
		}
		return err
	})
	return out, err
}

// SecurityReports reads the provenance of the reports in a scope without their findings.
func (u *UseCase) SecurityReports(ctx context.Context, org entity.ID, scope entity.SecurityScope) ([]entity.ReportEvidence, error) {
	var out []entity.ReportEvidence
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.readSecurityReports(ctx, org, scope)
		return err
	})
	return out, err
}

func (u *UseCase) readSecurityReports(ctx context.Context, org entity.ID, scope entity.SecurityScope) ([]entity.ReportEvidence, error) {
	reports, err := u.d.Repo.ReadReportEvidence(ctx, org, entity.ReportScope{AreaID: scope.AreaID, SHA: scope.SHA, KnownSeq: scope.KnownSeq})
	if err != nil {
		return nil, err
	}
	reports = slices.DeleteFunc(reports, func(r entity.ReportEvidence) bool {
		return !sameReportComponent(r.ComponentID, scope.ComponentID) || !securityReportKind(r.Kind)
	})
	return currentSecurityReports(reports), nil
}

// currentSecurityReports receives observations in descending knowledge order from ReadReportEvidence.
func currentSecurityReports(reports []entity.ReportEvidence) []entity.ReportEvidence {
	type stream struct {
		kind, language, format, producer, mode, profile, scope string
		secretScope, historyFrom, historyTo                    string
	}
	selected := make(map[stream]bool, len(reports))
	return slices.DeleteFunc(reports, func(r entity.ReportEvidence) bool {
		if r.Producer == "" || r.ScopeHash == "" {
			return false
		}
		key := stream{
			kind: r.Kind, language: r.Language, format: r.Format, producer: r.Producer,
			mode: r.Mode, profile: r.ProfileID, scope: r.ScopeHash, secretScope: r.SecretScope,
		}
		if r.SecretScope == entity.SecretScopeDiff {
			key.historyFrom, key.historyTo = r.HistoryFromSHA, r.HistoryToSHA
		}
		accepted, found := selected[key]
		if !found || (!accepted && r.RejectedReason == "") {
			// An invalid replacement does not invalidate the last accepted report.
			selected[key] = r.RejectedReason == ""
			return false
		}
		return true
	})
}

// ReceiptSecurityScopes lists every security scope affected by the normalized receipt.
func (u *UseCase) ReceiptSecurityScopes(ctx context.Context, org, receipt entity.ID) ([]entity.SecurityScope, error) {
	var out []entity.SecurityScope
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReceiptSecurityScopes(ctx, org, receipt)
		return err
	})
	return out, err
}

func sameReportComponent(a, b *entity.ID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func securityReportKind(kind string) bool {
	switch kind {
	case entity.AnalysisSAST, entity.AnalysisSCA, entity.AnalysisIaC, entity.AnalysisSecrets:
		return true
	default:
		return false
	}
}
