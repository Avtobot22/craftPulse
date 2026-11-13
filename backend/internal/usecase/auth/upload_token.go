package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

func (u *UseCase) uploadAccess(ctx context.Context, s entity.Subject, project entity.ID, operation string) error {
	if project.IsZero() {
		return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/project_id", Code: entity.FieldRequired}}}
	}
	_, err := u.d.Access.Authorize(ctx, s, entity.AccessRequest{Operation: operation, Action: "upload_tokens.manage", Area: entity.AreaRef{Kind: entity.AreaProject, ID: project}})
	return err
}

func (u *UseCase) CreateUploadToken(ctx context.Context, s entity.Subject, project entity.ID) (entity.UploadToken, entity.Secret, error) {
	if err := u.uploadAccess(ctx, s, project, "createUploadToken"); err != nil {
		return entity.UploadToken{}, "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return entity.UploadToken{}, "", err
	}
	value := "cpt_" + base64.RawURLEncoding.EncodeToString(b)
	var token entity.UploadToken
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		token, err = u.d.Tokens.CreateUploadToken(ctx, s.OrganizationID, project, s.UserID, hashToken(value), u.d.Clock.Now())
		if err != nil {
			return err
		}
		return u.uploadAudit(ctx, s, token, "createUploadToken", "")
	})
	if err != nil {
		return entity.UploadToken{}, "", err
	}
	return token, entity.Secret(value), nil
}

func (u *UseCase) ListUploadTokens(ctx context.Context, s entity.Subject, project entity.ID, limit int, after *entity.AuditKey) (entity.UploadTokenPage, error) {
	if err := u.uploadAccess(ctx, s, project, "listUploadTokens"); err != nil {
		return entity.UploadTokenPage{}, err
	}
	if limit < 1 || limit > 200 {
		return entity.UploadTokenPage{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "limit", Code: entity.FieldOutOfRange}}}
	}
	var items []entity.UploadToken
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		items, err = u.d.Tokens.PageUploadTokens(ctx, s.OrganizationID, project, limit+1, after)
		return err
	})
	if err != nil {
		return entity.UploadTokenPage{}, err
	}
	out := entity.UploadTokenPage{Items: items}
	if len(items) > limit {
		out.Items = items[:limit]
		last := out.Items[limit-1]
		out.Next = &entity.AuditKey{OccurredAt: last.CreatedAt, ID: last.ID}
	}
	return out, nil
}

func (u *UseCase) RevokeUploadToken(ctx context.Context, s entity.Subject, id entity.ID) error {
	token, err := u.d.Tokens.UploadToken(ctx, s.OrganizationID, id)
	if err != nil {
		return err
	}
	if err := u.uploadAccess(ctx, s, token.ProjectID, "revokeUploadToken"); err != nil {
		return err
	}
	return u.revokeUpload(ctx, s, token, "")
}

func (u *UseCase) AuthenticateUpload(ctx context.Context, value string) (entity.Subject, error) {
	if !validUploadValue(value) {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	token, err := u.d.Tokens.UploadTokenByHash(ctx, hashToken(value))
	if errors.Is(err, entity.ErrNotFound) {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	if err != nil {
		return entity.Subject{}, err
	}
	if token.RevokedAt != nil {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	return entity.Subject{OrganizationID: token.OrganizationID, ProjectID: token.ProjectID, UploadTokenID: token.ID}, nil
}

func (u *UseCase) RejectExposedUpload(ctx context.Context, value string) error {
	if !validUploadValue(value) {
		return nil
	}
	token, err := u.d.Tokens.UploadTokenByHash(ctx, hashToken(value))
	if errors.Is(err, entity.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return u.revokeUpload(ctx, entity.Subject{OrganizationID: token.OrganizationID, ProjectID: token.ProjectID, UploadTokenID: token.ID}, token, "token_in_url")
}

func (u *UseCase) revokeUpload(ctx context.Context, s entity.Subject, t entity.UploadToken, reason string) error {
	return u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		changed, err := u.d.Tokens.RevokeUploadToken(ctx, s.OrganizationID, t.ID, u.d.Clock.Now())
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return u.uploadAudit(ctx, s, t, "revokeUploadToken", reason)
	})
}

func (u *UseCase) uploadAudit(ctx context.Context, s entity.Subject, t entity.UploadToken, operation, reason string) error {
	kind, id := entity.ActorUser, s.UserID
	if !s.UploadTokenID.IsZero() {
		kind, id = entity.ActorUploadToken, s.UploadTokenID
	}
	return u.d.Auditor.Record(ctx, entity.AuditEvent{OrganizationID: s.OrganizationID, OccurredAt: u.d.Clock.Now(), ActorKind: kind, ActorID: id, Operation: operation, ObjectKind: "upload_token", ObjectID: t.ID, AreaID: t.ProjectID, Outcome: entity.AuditSucceeded, ReasonCode: reason, CorrelationID: entity.ID(logger.CorrelationID(ctx))})
}

func validUploadValue(value string) bool {
	if len(value) != 47 || !strings.HasPrefix(value, "cpt_") {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value[4:])
	return err == nil && len(b) == 32
}
