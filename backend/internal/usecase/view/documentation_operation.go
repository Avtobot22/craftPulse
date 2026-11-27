package view

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

type documentationParameters struct {
	RootID   entity.ID `json:"root_id"`
	AfterSeq int64     `json:"after_seq"`
}

// RequestDocumentationCheck records a wait for client CI for a confirmed root.
func (u *UseCase) RequestDocumentationCheck(ctx context.Context, s entity.Subject, scope entity.Scope, service, root entity.ID) (entity.Operation, error) {
	org := s.OrganizationID
	id, err := u.d.NewID()
	if err != nil {
		return entity.Operation{}, err
	}
	hash := sha256.Sum256(root[:])
	now := u.d.Clock.Now()
	op := entity.Operation{
		ID: id, OrganizationID: org, RequestedBy: s.UserID, Kind: entity.OperationDocumentationCheck,
		ResourceKind: "service", ResourceID: service, Action: "docs.manage", RequestHash: hash[:],
		Status: entity.OperationQueued, CreatedAt: now, UpdatedAt: now,
	}
	var out entity.Operation
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		roots, err := u.d.Documentation.Roots.ListRoots(ctx, s, scope, service)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(roots.Roots, func(r entity.DocumentationRoot) bool { return r.ID == root }) {
			return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/root_id", Code: entity.FieldOutsideScope}}}
		}
		if active, found, err := u.activeDocumentationCheck(ctx, op, roots.Roots); err != nil || found {
			out = active
			return err
		}
		seq, _, err := u.d.Documentation.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		op.Parameters, err = json.Marshal(documentationParameters{RootID: root, AfterSeq: seq})
		if err != nil {
			return fmt.Errorf("view: encode documentation check: %w", err)
		}
		created, err := u.d.Repo.CreateOperation(ctx, op)
		if err != nil || created {
			out = op
			return err
		}
		// A concurrent requester may have inserted after ActiveOperation saw no row.
		var found bool
		out, found, err = u.activeDocumentationCheck(ctx, op, roots.Roots)
		if err == nil && !found {
			return entity.ErrStateConflict
		}
		return err
	})
	return out, err
}

func (u *UseCase) activeDocumentationCheck(ctx context.Context, requested entity.Operation, roots []entity.DocumentationRoot) (entity.Operation, bool, error) {
	op, found, err := u.d.Repo.ActiveOperation(ctx, requested)
	if err != nil || !found {
		return op, found, err
	}
	op, err = u.refreshDocumentationCheck(ctx, op, roots)
	if err != nil || op.Done() {
		return op, false, err
	}
	if !bytes.Equal(op.RequestHash, requested.RequestHash) {
		return op, true, entity.ErrStateConflict
	}
	return op, true, nil
}

func (u *UseCase) pollDocumentationCheck(ctx context.Context, s entity.Subject, scope entity.Scope, op entity.Operation) (entity.Operation, error) {
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		set, err := u.d.Documentation.Roots.ListRoots(ctx, s, scope, op.ResourceID)
		if err != nil {
			return err
		}
		active, found, err := u.d.Repo.ActiveOperation(ctx, op)
		if err != nil {
			return err
		}
		if !found || active.ID != op.ID {
			op, found, err = u.d.Repo.Operation(ctx, s.OrganizationID, op.ID)
			if err == nil && !found {
				return entity.ErrNotFound
			}
			return err
		}
		op, err = u.refreshDocumentationCheck(ctx, active, set.Roots)
		return err
	})
	return op, err
}

func (u *UseCase) refreshDocumentationCheck(ctx context.Context, op entity.Operation, roots []entity.DocumentationRoot) (entity.Operation, error) {
	var p documentationParameters
	if err := json.Unmarshal(op.Parameters, &p); err != nil {
		return op, fmt.Errorf("view: decode documentation check: %w", err)
	}
	i := slices.IndexFunc(roots, func(root entity.DocumentationRoot) bool { return root.ID == p.RootID })
	now := u.d.Clock.Now()
	if i < 0 {
		op.Status, op.UpdatedAt, op.CompletedAt = entity.OperationFailed, now, &now
		op.ErrorCode, op.CorrelationID = entity.OperationAccessLost, new(entity.ID(logger.CorrelationID(ctx)))
		return op, u.d.Repo.UpdateOperation(ctx, op)
	}
	d, org := u.d.Documentation, op.OrganizationID
	seq, _, err := d.Tenants.Clock(ctx, org)
	if err != nil {
		return op, err
	}
	finished, err := d.Reports.DocumentationCheckCompleted(ctx, org, roots[i], seq, p.AfterSeq, op.CreatedAt)
	if err != nil || !finished {
		return op, err
	}
	op.Status, op.UpdatedAt, op.CompletedAt = entity.OperationSucceeded, now, &now
	op.ResultURI = "/api/v1/services/" + op.ResourceID.String() + "/documents"
	return op, u.d.Repo.UpdateOperation(ctx, op)
}
