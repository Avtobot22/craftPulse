package process

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_operationConfirmTarget = "process.target.confirm"
	_maxLabel               = 200
)

// CheckTargetDraft checks the fields of a draft of a target (FR-019): the group is required, since a target is
// not applied without a size, the duration is positive, and a historical p85 names its revision and period.
// Errors of fields are *entity.ValidationError.
func (*UseCase) CheckTargetDraft(d entity.TargetDraft) error {
	verr := &entity.ValidationError{}
	checkLabel(verr, "/task_type", d.TaskType)
	checkLabel(verr, "/size_label", d.SizeLabel)
	if d.DurationSeconds <= 0 {
		verr.Add("/duration_seconds", entity.FieldOutOfRange, "the target is longer than zero")
	}
	switch d.Basis {
	case entity.TargetHistoricalP85:
		if d.BasisRevisionID == nil {
			verr.Add("/basis_revision_id", entity.FieldRequired, "historical_p85 names the revision of its baseline")
		}
		switch {
		case d.BasisPeriod == nil:
			verr.Add("/basis_period", entity.FieldRequired, "historical_p85 names the period of its baseline")
		case !d.BasisPeriod.From.Before(d.BasisPeriod.To):
			verr.Add("/basis_period", entity.FieldOutOfRange, "from is before to")
		}
	case entity.TargetManual:
		if d.BasisRevisionID != nil {
			verr.Add("/basis_revision_id", entity.FieldNotAllowedValue, "a manual target has no revision, send null")
		}
		if d.BasisPeriod != nil {
			verr.Add("/basis_period", entity.FieldNotAllowedValue, "a manual target has no period, send null")
		}
	default:
		verr.Add("/basis", entity.FieldNotAllowedValue, "allowed: "+entity.TargetManual+", "+entity.TargetHistoricalP85)
	}
	if len(verr.Fields) > 0 {
		return verr
	}
	return nil
}

// ListTargetVersions pages the versions of the targets of a team newest first; with a type and a size it reads
// one group and Latest is its last version for If-Match.
func (u *UseCase) ListTargetVersions(ctx context.Context, s entity.Subject, teamID entity.ID, f entity.TargetVersionFilter) (entity.TargetVersionPage, error) {
	org := s.OrganizationID
	var page entity.TargetVersionPage
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		if f.TaskType != nil && f.SizeLabel != nil {
			latest, err := u.d.Repo.LatestTargetVersionNo(ctx, org, teamID, entity.TargetGroup{TaskType: *f.TaskType, SizeLabel: *f.SizeLabel})
			if err != nil {
				return err
			}
			page.Latest = &latest
		}
		items, err := u.d.Repo.PageTargetVersions(ctx, org, teamID, f)
		if err != nil {
			return err
		}
		if len(items) > f.Limit {
			items = items[:f.Limit]
			next := items[len(items)-1].KnownSeq
			page.Next = &next
		}
		page.Items = items
		return nil
	})
	return page, err
}

// ConfirmTargetVersion saves the draft as version baseVersion+1 of its group with the recount of the series of
// the team in the transaction of the caller, which calc opens (TZ 10.3, rule 2). It follows CreateMappingVersion: a repeat
// of the key first, then If-Match, then a draft equal to the version in force. basis is the baseline of a
// historical p85 as calc read it from the revision.
func (u *UseCase) ConfirmTargetVersion(ctx context.Context, s entity.Subject, teamID entity.ID, baseVersion int, key entity.ID,
	d entity.TargetDraft, basis *entity.TargetBasis,
) (entity.TargetVersion, entity.SaveResult, error) {
	if err := u.CheckTargetDraft(d); err != nil {
		return entity.TargetVersion{}, 0, err
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.TargetVersion{}, 0, errorf("new target version id: %w", err)
	}
	org := s.OrganizationID
	seq, err := u.d.Tenants.NextSeq(ctx, org)
	if err != nil {
		return entity.TargetVersion{}, 0, err
	}
	saved, err := u.d.Repo.TargetVersionByConfirmation(ctx, org, key)
	if err == nil {
		if saved.TeamID != teamID || saved.ConfirmedBy != s.UserID || saved.TargetGroup != d.TargetGroup || !sameTarget(&saved, d, basis) {
			return entity.TargetVersion{}, 0, entity.ErrIdempotencyKeyReused
		}
		return saved, entity.SaveRepeated, nil
	}
	if !isNotFound(err) {
		return entity.TargetVersion{}, 0, err
	}
	latest, err := u.d.Repo.LatestTargetVersionNo(ctx, org, teamID, d.TargetGroup)
	if err != nil {
		return entity.TargetVersion{}, 0, err
	}
	if latest != baseVersion {
		return entity.TargetVersion{}, 0, entity.ErrVersionConflict
	}
	if latest > 0 {
		current, err := u.d.Repo.CurrentTargetVersion(ctx, org, teamID, d.TargetGroup)
		if err != nil {
			return entity.TargetVersion{}, 0, err
		}
		if sameTarget(&current, d, basis) {
			return current, entity.SaveUnchanged, nil
		}
	}
	v := entity.TargetVersion{
		ID: id, TeamID: teamID, TargetGroup: d.TargetGroup, VersionNo: latest + 1, BaseVersionNo: baseOf(latest),
		DurationSeconds: d.DurationSeconds, Basis: d.Basis, BasisOf: basis, KnownSeq: seq,
		Confirmation: entity.Confirmation{ConfirmedBy: s.UserID, ConfirmedAt: u.d.Clock.Now(), ConfirmationID: key},
	}
	if err := u.d.Repo.InsertTargetVersion(ctx, org, v); err != nil {
		return entity.TargetVersion{}, 0, err
	}
	if err := u.record(ctx, s, _operationConfirmTarget, "target_version", v.ID, teamID, latest, v.Confirmation); err != nil {
		return entity.TargetVersion{}, 0, err
	}
	if err := u.recount(ctx, org, teamID); err != nil {
		return entity.TargetVersion{}, 0, err
	}
	return v, entity.SaveInserted, nil
}

// TargetVersionsAt are the versions of the targets of a team in force at the knowledge number seq, one per group,
// for T10 and T11 of a revision (TZ 10.6).
func (u *UseCase) TargetVersionsAt(ctx context.Context, orgID, teamID entity.ID, seq int64) ([]entity.TargetVersion, error) {
	var out []entity.TargetVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.TargetVersionsAt(ctx, orgID, teamID, seq)
		return err
	})
	return out, err
}

func checkLabel(verr *entity.ValidationError, field, label string) {
	switch {
	case strings.TrimSpace(label) == "":
		verr.Add(field, entity.FieldRequired, "")
	case utf8.RuneCountInString(label) > _maxLabel:
		verr.Add(field, entity.FieldTooLong, "")
	}
}

// sameTarget tells whether a version holds the duration and the basis of the draft.
func sameTarget(v *entity.TargetVersion, d entity.TargetDraft, basis *entity.TargetBasis) bool {
	if v.DurationSeconds != d.DurationSeconds || v.Basis != d.Basis || (v.BasisOf == nil) != (basis == nil) {
		return false
	}
	if basis == nil {
		return true
	}
	b := v.BasisOf
	return b.RevisionID == basis.RevisionID && b.ValueSeconds == basis.ValueSeconds && b.N == basis.N &&
		b.Window.From.Equal(basis.Window.From) && b.Window.To.Equal(basis.Window.To)
}
