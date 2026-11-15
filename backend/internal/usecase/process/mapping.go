package process

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// ListMappingVersions pages the versions of the mapping of a team newest first; Latest is for If-Match.
func (u *UseCase) ListMappingVersions(ctx context.Context, s entity.Subject, teamID entity.ID, p entity.VersionPageRequest) (entity.MappingVersionPage, error) {
	org := s.OrganizationID
	var page entity.MappingVersionPage
	err := u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		if page.Latest, err = u.d.Repo.LatestMappingVersionNo(ctx, org, teamID); err != nil {
			return err
		}
		items, err := u.d.Repo.PageMappingVersions(ctx, org, teamID, p)
		if err != nil {
			return err
		}
		if len(items) > p.Limit {
			items = items[:p.Limit]
			next := items[len(items)-1].VersionNo
			page.Next = &next
		}
		page.Items = items
		return nil
	})
	return page, err
}

// CreateMappingVersion confirms the draft as version baseVersion+1 of the mapping of a team (FR-017). A repeat of
// the key returns the version it saved before If-Match is checked; the same key with another request is
// refused. A form on a replaced version is a version conflict (FR-008), and a draft equal to the current
// version changes nothing (FR-007). created tells a new or repeated version from the current one returned.
func (u *UseCase) CreateMappingVersion(ctx context.Context, s entity.Subject, teamID entity.ID, baseVersion int, key entity.ID,
	d entity.MappingDraft,
) (v entity.MappingVersion, created bool, err error) {
	org := s.OrganizationID
	check, err := u.validateDraft(ctx, org, teamID, d)
	if err != nil {
		return entity.MappingVersion{}, false, err
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.MappingVersion{}, false, errorf("new mapping version id: %w", err)
	}
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		created = false
		seq, err := u.d.Tenants.NextSeq(ctx, org)
		if err != nil {
			return err
		}
		saved, err := u.d.Repo.MappingVersionByConfirmation(ctx, org, key)
		if err == nil {
			if saved.TeamID != teamID || saved.ConfirmedBy != s.UserID || !bytes.Equal(saved.ContentHash, check.hash) {
				return entity.ErrIdempotencyKeyReused
			}
			v, created = saved, true
			return nil
		}
		if !isNotFound(err) {
			return err
		}
		latest, err := u.d.Repo.LatestMappingVersionNo(ctx, org, teamID)
		if err != nil {
			return err
		}
		if latest != baseVersion {
			return entity.ErrVersionConflict
		}
		if latest > 0 {
			current, err := u.d.Repo.MappingVersionAt(ctx, org, teamID, math.MaxInt64)
			if err != nil {
				return err
			}
			if bytes.Equal(current.ContentHash, check.hash) {
				v = current
				return nil
			}
		}
		v = entity.MappingVersion{
			ID: id, TeamID: teamID, VersionNo: latest + 1, BaseVersionNo: baseOf(latest), ContentHash: check.hash,
			Timezone: check.draft.Timezone, CalendarVersionID: check.calendarID, CalendarVersionNo: check.draft.CalendarVersionNo,
			KnownSeq: seq, Confirmation: entity.Confirmation{ConfirmedBy: s.UserID, ConfirmedAt: u.d.Clock.Now(), ConfirmationID: key},
			Rules: check.draft.Rules,
		}
		if err := u.d.Repo.InsertMappingVersion(ctx, org, v); err != nil {
			return err
		}
		created = true
		if err := u.record(ctx, s, _operationConfirmMapping, "mapping_version", v.ID, teamID, latest, v.Confirmation); err != nil {
			return err
		}
		return u.recount(ctx, org, teamID)
	})
	if err != nil {
		return entity.MappingVersion{}, false, err
	}
	return v, created, nil
}

// MappingAt is the version of the mapping of a team in force at the knowledge number seq, the slice of a
// revision of calc (TZ 10.6, 12.6); false when the team had none.
func (u *UseCase) MappingAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, bool, error) {
	var out entity.MappingVersion
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		out, err = u.d.Repo.MappingVersionAt(ctx, orgID, teamID, seq)
		return err
	})
	if isNotFound(err) {
		return entity.MappingVersion{}, false, nil
	}
	return out, err == nil, err
}

// LastSettingsSeq is the knowledge number of the last version of the settings of a team, by which a job of calc
// checks whether its input changed (TZ 10.6); 0 when the team has none.
func (u *UseCase) LastSettingsSeq(ctx context.Context, orgID, teamID entity.ID) (int64, error) {
	var seq int64
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		seq, err = u.d.Repo.LastSettingsSeq(ctx, orgID, teamID)
		return err
	})
	return seq, err
}

// record writes the confirmation of a version in the audit: the author, the time, the team and the numbers (DR-004).
func (u *UseCase) record(ctx context.Context, s entity.Subject, operation, kind string, id, teamID entity.ID, base int,
	c entity.Confirmation,
) error {
	return u.d.Auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID,
		OccurredAt:     c.ConfirmedAt,
		ActorKind:      entity.ActorUser,
		ActorID:        s.UserID,
		Operation:      operation,
		ObjectKind:     kind,
		ObjectID:       id,
		AreaID:         teamID,
		Outcome:        entity.AuditSucceeded,
		BaseVersion:    strconv.Itoa(base),
		NewVersion:     strconv.Itoa(base + 1),
		CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
	})
}

func baseOf(latest int) *int {
	if latest == 0 {
		return nil
	}
	return &latest
}

func isNotFound(err error) bool { return errors.Is(err, entity.ErrNotFound) }
