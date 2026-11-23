package change

import (
	"context"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_exceptionCreate  = "createException"
	_exceptionVersion = "createExceptionVersion"
)

// Decisions is the writer and effective-history reader that change uses after resolving canonical findings.
type Decisions interface {
	CheckDraft(d *entity.ExceptionDraft) error
	Get(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.ExceptionView, error)
	Save(ctx context.Context, s entity.Subject, scope entity.Scope, cmd entity.ExceptionWrite) (entity.ExceptionView, error)
	ActiveAt(ctx context.Context, org, area entity.ID, seq int64, at time.Time) ([]entity.ExceptionVersion, error)
}

// FindingRights checks access to the source area and the finding class.
type FindingRights interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
	VisibleFindings(ctx context.Context, s entity.Subject, scope entity.Scope, operation string, targets []entity.FindingAccess) (map[entity.FindingAccess]bool, error)
}

// CreateException resolves a finding or rule before asking decision to store its version.
func (u *UseCase) CreateException(ctx context.Context, s entity.Subject, scope entity.Scope, cmd entity.ExceptionWrite) (entity.ExceptionView, error) {
	if err := u.d.Decisions.CheckDraft(&cmd.Draft); err != nil {
		return entity.ExceptionView{}, err
	}
	op := _exceptionCreate
	if !cmd.ID.IsZero() {
		op = _exceptionVersion
	}
	read, err := u.d.FindingRights.Authorize(ctx, s, entity.AccessRequest{Operation: op, Action: "findings.read"})
	if err != nil {
		return entity.ExceptionView{}, err
	}
	if err := u.checkExceptionArea(ctx, s, read, &cmd); err != nil {
		return entity.ExceptionView{}, err
	}
	kind, err := u.resolveExceptionScope(ctx, s, read, cmd.Draft.Scope, op)
	if err != nil {
		return entity.ExceptionView{}, err
	}
	target := entity.FindingAccess{AreaID: cmd.Draft.Scope.AreaID, Kind: kind, WholeArea: cmd.Draft.Scope.ComponentID.IsZero()}
	if err := u.mayWriteException(ctx, s, scope, op, target); err != nil {
		return entity.ExceptionView{}, err
	}
	cmd.FindingKind = kind
	return u.d.Decisions.Save(ctx, s, scope, cmd)
}

func (u *UseCase) checkExceptionArea(ctx context.Context, s entity.Subject, read entity.Scope, cmd *entity.ExceptionWrite) error {
	if cmd.ID.IsZero() {
		return nil
	}
	old, err := u.d.Decisions.Get(ctx, s, read, cmd.ID)
	if err != nil {
		return err
	}
	if old.Current.Scope.AreaID != cmd.Draft.Scope.AreaID {
		return outsideExceptionScope("/scope/area_id")
	}
	return nil
}

func (u *UseCase) mayWriteException(ctx context.Context, s entity.Subject, scope entity.Scope, op string, target entity.FindingAccess) error {
	narrow := target
	narrow.WholeArea = false
	targets := []entity.FindingAccess{target}
	if target.WholeArea {
		targets = append(targets, narrow)
	}
	allowed, err := u.d.FindingRights.VisibleFindings(ctx, s, scope, op, targets)
	if err != nil {
		return err
	}
	if !allowed[target] {
		if target.WholeArea && allowed[narrow] {
			return outsideExceptionScope("/scope")
		}
		return entity.ErrForbidden
	}
	return nil
}

func (u *UseCase) resolveExceptionScope(ctx context.Context, s entity.Subject, read entity.Scope, requested entity.ExceptionScope, op string) (string, error) {
	org := s.OrganizationID
	if !requested.FindingID.IsZero() {
		var identity entity.FindingIdentity
		err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
			var err error
			identity, err = u.d.Repo.FindingIdentity(ctx, org, requested.FindingID)
			return err
		})
		if err != nil {
			return "", err
		}
		target := entity.FindingAccess{AreaID: identity.AreaID, Kind: identity.Kind}
		allowed, err := u.d.FindingRights.VisibleFindings(ctx, s, read, op, []entity.FindingAccess{target})
		if err != nil {
			return "", err
		}
		if !allowed[target] {
			return "", entity.ErrNotFound
		}
		if target.AreaID != requested.AreaID {
			return "", outsideExceptionScope("/scope/area_id")
		}
	}
	var kinds []string
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		kinds, err = u.d.Repo.FindingKindsForException(ctx, org, requested)
		return err
	})
	if err != nil {
		return "", err
	}
	if len(kinds) == 0 && !requested.FindingID.IsZero() {
		return "", outsideExceptionScope("/scope")
	}
	targets := make([]entity.FindingAccess, len(kinds))
	for i, kind := range kinds {
		targets[i] = entity.FindingAccess{AreaID: requested.AreaID, Kind: kind}
	}
	visible, err := u.d.FindingRights.VisibleFindings(ctx, s, read, op, targets)
	if err != nil {
		return "", err
	}
	var result string
	for _, target := range targets {
		if !visible[target] {
			continue
		}
		if result != "" {
			return "", &entity.ValidationError{Fields: []entity.FieldError{
				{Field: "/scope/rule_id", Code: entity.FieldNotAllowedValue, Detail: "select a finding when the rule has several finding kinds"},
			}}
		}
		result = target.Kind
	}
	if result == "" {
		return "", entity.ErrNotFound
	}
	return result, nil
}

func outsideExceptionScope(field string) error {
	return &entity.ValidationError{Fields: []entity.FieldError{{Field: field, Code: entity.FieldOutsideScope}}}
}

// RunExceptionRecount records the effective exception inputs for change without rewriting findings or baselines.
func (u *UseCase) RunExceptionRecount(ctx context.Context, job entity.ChangeRecountJob, complete Complete) error {
	org := job.OrganizationID
	return u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		seq, _, err := u.d.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		at := u.d.Clock.Now().UTC()
		active, err := u.d.Decisions.ActiveAt(ctx, org, job.AreaID, seq, at)
		if err != nil {
			return err
		}
		versions := make([]entity.ID, len(active))
		for i := range active {
			versions[i] = active[i].ID
		}
		if err := u.d.Repo.ProjectExceptions(ctx, org, job.AreaID, seq, at, versions); err != nil {
			return err
		}
		return complete(ctx)
	})
}
