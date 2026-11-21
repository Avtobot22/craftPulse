package normalize

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// ReceiptReports are the reports a receipt gave with their findings, for change to identify them; the caller
// reads them in its own organization transaction or in a new one.
func (u *UseCase) ReceiptReports(ctx context.Context, orgID, receiptID entity.ID) ([]entity.ReportFindings, error) {
	var out []entity.ReportFindings
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReceiptReports(ctx, orgID, receiptID)
		return err
	})
	return out, err
}

// ReportsByID are the reports of ids without their findings.
func (u *UseCase) ReportsByID(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ReportFindings, error) {
	var out []entity.ReportFindings
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReportsByID(ctx, orgID, ids)
		return err
	})
	return out, err
}

// LastBranchReports gives the last report of every query: of its component, language and kind of analysis on its
// branch; a query without a report is absent.
func (u *UseCase) LastBranchReports(ctx context.Context, orgID entity.ID, queries []entity.BranchReportQuery) ([]entity.ReportFindings, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	var components []entity.ID
	var refs []string
	for _, q := range queries {
		if !slices.Contains(components, q.ComponentID) {
			components = append(components, q.ComponentID)
		}
		if !slices.Contains(refs, q.RefName) {
			refs = append(refs, q.RefName)
		}
	}
	var list []entity.ReportFindings
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		list, err = u.d.Repo.LastBranchReports(ctx, orgID, components, refs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(r entity.ReportFindings) bool {
		return !slices.ContainsFunc(queries, func(q entity.BranchReportQuery) bool {
			return r.ComponentID != nil && *r.ComponentID == q.ComponentID && r.Language == q.Language && r.Kind == q.Kind && r.RefName == q.RefName
		})
	}), nil
}

// DefaultBranches are the main branches of the repositories that name one.
func (u *UseCase) DefaultBranches(ctx context.Context, orgID entity.ID, repositories []entity.ID) (map[entity.ID]string, error) {
	var out map[entity.ID]string
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.DefaultBranches(ctx, orgID, repositories)
		return err
	})
	return out, err
}
