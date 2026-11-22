package normalize

import (
	"context"

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
