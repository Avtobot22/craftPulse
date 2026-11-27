package normalize

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) DocumentsAt(ctx context.Context, org, area entity.ID, prefix, sha string, seq int64) ([]entity.DocumentVersion, error) {
	var out []entity.DocumentVersion
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.DocumentsAt(ctx, org, area, prefix, sha, seq)
		return err
	})
	return out, err
}

func (u *UseCase) DocumentationReportsAt(ctx context.Context, org entity.ID, root *entity.DocumentationRoot, sha string, seq int64) ([]entity.DocsReport, []entity.DocsFinding, error) {
	var reports []entity.DocsReport
	var findings []entity.DocsFinding
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		reports, findings, err = u.d.Repo.DocumentationReportsAt(ctx, org, root, sha, seq)
		return err
	})
	return reports, findings, err
}

func (u *UseCase) ReceiptDocumentationRoots(ctx context.Context, org, receipt entity.ID) ([]entity.DocsReceiptRoot, error) {
	var out []entity.DocsReceiptRoot
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.ReceiptDocumentationRoots(ctx, org, receipt)
		return err
	})
	return out, err
}

// LatestDocumentationChecks reads profile observations in one confirmed root.
func (u *UseCase) LatestDocumentationChecks(ctx context.Context, org entity.ID, root entity.DocumentationRoot, through, after int64) ([]entity.DocumentationCheck, error) {
	var out []entity.DocumentationCheck
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.LatestDocumentationChecks(ctx, org, root, through, after)
		return err
	})
	return out, err
}

// DocumentationCheckCompleted reports whether a new pipeline finished the checks of the requested root.
func (u *UseCase) DocumentationCheckCompleted(ctx context.Context, org entity.ID, root entity.DocumentationRoot, through, after int64, requestedAt time.Time) (bool, error) {
	var complete bool
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		complete, err = u.d.Repo.DocumentationCheckCompleted(ctx, org, root, through, after, requestedAt)
		return err
	})
	return complete, err
}

func (u *UseCase) WikiDocumentsAt(ctx context.Context, org, area entity.ID, prefix string, seq int64) ([]entity.DocumentVersion, error) {
	var out []entity.DocumentVersion
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.WikiDocumentsAt(ctx, org, area, prefix, seq)
		return err
	})
	return out, err
}

func (u *UseCase) DocumentationAreaKinds(ctx context.Context, org entity.ID, ids []entity.ID) (map[entity.ID]entity.ScopeKind, error) {
	var out map[entity.ID]entity.ScopeKind
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.DocumentationAreaKinds(ctx, org, ids)
		return err
	})
	return out, err
}

func (u *UseCase) CurrentWikiVersions(ctx context.Context, org, area entity.ID, ids []string) (map[entity.ObjectKey]entity.SyncObjectVersion, error) {
	var out map[entity.ObjectKey]entity.SyncObjectVersion
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.CurrentWikiVersions(ctx, org, area, ids)
		return err
	})
	return out, err
}

func (u *UseCase) WikiVersionBoundary(ctx context.Context, org, area entity.ID, seq int64, asOf time.Time) (*time.Time, error) {
	var out *time.Time
	err := u.d.Tx.JoinOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.WikiVersionBoundary(ctx, org, area, seq, asOf)
		return err
	})
	return out, err
}
