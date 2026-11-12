package org

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_operationReplaceRoles = "user.roles.replace"
	_operationBlock        = "user.block"
	_operationUnblock      = "user.unblock"
	_operationEndSessions  = "user.sessions.end"
)

var _roles = map[entity.Role]bool{
	entity.RoleAdministrator: true, entity.RoleDevLead: true, entity.RoleProjectManager: true, entity.RoleTechLead: true,
	entity.RoleDeveloper: true, entity.RoleProcessOwner: true, entity.RoleServiceOwner: true,
	entity.RoleSecurityAccess: true, entity.RoleWikiAccess: true,
}

// Credentials is the auth module: sessions and API keys of a user, changed in the caller's transaction.
type Credentials interface {
	EndUserSessions(ctx context.Context, orgID, userID entity.ID, reason entity.SessionEndReason, at time.Time) (int64, error)
	RevokeUserAPIKeys(ctx context.Context, orgID, userID entity.ID, at time.Time) (int64, error)
}

func (u *UseCase) ListUsers(ctx context.Context, s entity.Subject, req entity.UserListRequest) (entity.UserPage, error) {
	var page entity.UserPage
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		users, err := u.orgs.PageUsers(ctx, s.OrganizationID, req)
		if err != nil {
			return err
		}
		if len(users) > req.Limit {
			users = users[:req.Limit]
			last := users[len(users)-1]
			page.Next = &entity.UserKey{Name: last.DisplayName, CreatedAt: last.CreatedAt, ID: last.ID}
		}
		page.Items = users
		return nil
	})
	return page, err
}

// UserRoles returns the active roles of a user of the organization; another one is entity.ErrNotFound.
func (u *UseCase) UserRoles(ctx context.Context, s entity.Subject, userID entity.ID) (entity.RoleSet, error) {
	var set entity.RoleSet
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		if _, err := u.orgs.User(ctx, s.OrganizationID, userID); err != nil {
			return err
		}
		var err error
		set, err = u.roleSet(ctx, s.OrganizationID, userID)
		return err
	})
	return set, err
}

// ReplaceUserRoles replaces the whole role set confirmed against baseVersion. The change applies to the
// next request of the user, since roles are read on every request (TZ 7.2).
func (u *UseCase) ReplaceUserRoles(ctx context.Context, s entity.Subject, userID entity.ID, baseVersion int64,
	roles []entity.RoleAssignment,
) (entity.RoleSet, error) {
	orgID := s.OrganizationID
	var set entity.RoleSet
	err := u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		if _, err := u.orgs.LockUser(ctx, orgID, userID); err != nil {
			return err
		}
		current, err := u.roleSet(ctx, orgID, userID)
		if err != nil {
			return err
		}
		if current.Version != baseVersion {
			return entity.ErrVersionConflict
		}
		if err := u.validateRoles(ctx, orgID, roles); err != nil {
			return err
		}
		revoked, assigned := diffRoles(current.Roles, roles)
		if len(revoked)+len(assigned) == 0 {
			set = current
			return nil
		}
		now := u.clock.Now()
		for _, r := range revoked {
			if err := u.orgs.RevokeRole(ctx, orgID, r.ID, s.UserID, now); err != nil {
				return err
			}
		}
		for _, r := range assigned {
			if err := u.orgs.AssignRole(ctx, orgID, userID, r, s.UserID, now); err != nil {
				return err
			}
		}
		if set, err = u.roleSet(ctx, orgID, userID); err != nil {
			return err
		}
		return u.auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: orgID,
			OccurredAt:     now,
			ActorKind:      entity.ActorUser,
			ActorID:        s.UserID,
			Operation:      _operationReplaceRoles,
			ObjectKind:     "user",
			ObjectID:       userID,
			Outcome:        entity.AuditSucceeded,
			BaseVersion:    strconv.FormatInt(current.Version, 10),
			NewVersion:     strconv.FormatInt(set.Version, 10),
			CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
			Details:        roleChange(current.Roles, set.Roles),
		})
	})
	return set, err
}

// BlockUser blocks the user and, in the same transaction, ends all sessions and disables the API keys
// (TZ 13.2). A repeat ends what has appeared since and changes nothing else.
func (u *UseCase) BlockUser(ctx context.Context, s entity.Subject, userID entity.ID) error {
	orgID := s.OrganizationID
	return u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		user, err := u.orgs.LockUser(ctx, orgID, userID)
		if err != nil {
			return err
		}
		now := u.clock.Now()
		changed := user.Status != entity.UserBlocked
		if changed {
			if err := u.orgs.SetUserStatus(ctx, orgID, userID, entity.UserBlocked, now); err != nil {
				return err
			}
		}
		sessions, err := u.credentials.EndUserSessions(ctx, orgID, userID, entity.SessionBlocked, now)
		if err != nil {
			return err
		}
		keys, err := u.credentials.RevokeUserAPIKeys(ctx, orgID, userID, now)
		if err != nil {
			return err
		}
		if !changed && sessions == 0 && keys == 0 {
			return nil
		}
		return u.recordUser(ctx, s, userID, _operationBlock, now, map[string]any{"ended_sessions": sessions, "revoked_api_keys": keys})
	})
}

// UnblockUser lifts the block; ended sessions and disabled keys stay as they are.
func (u *UseCase) UnblockUser(ctx context.Context, s entity.Subject, userID entity.ID) error {
	orgID := s.OrganizationID
	return u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		user, err := u.orgs.LockUser(ctx, orgID, userID)
		if err != nil {
			return err
		}
		if user.Status == entity.UserActive {
			return nil
		}
		now := u.clock.Now()
		if err := u.orgs.SetUserStatus(ctx, orgID, userID, entity.UserActive, time.Time{}); err != nil {
			return err
		}
		return u.recordUser(ctx, s, userID, _operationUnblock, now, nil)
	})
}

// EndUserSessions ends every session of the user; the next request with any of them gets 401.
func (u *UseCase) EndUserSessions(ctx context.Context, s entity.Subject, userID entity.ID) error {
	orgID := s.OrganizationID
	return u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		if _, err := u.orgs.LockUser(ctx, orgID, userID); err != nil {
			return err
		}
		now := u.clock.Now()
		ended, err := u.credentials.EndUserSessions(ctx, orgID, userID, entity.SessionAdmin, now)
		if err != nil || ended == 0 {
			return err
		}
		return u.recordUser(ctx, s, userID, _operationEndSessions, now, map[string]any{"ended_sessions": ended})
	})
}

// recordUser writes who changed whose account; session IDs never reach the record (TZ 13.8).
func (u *UseCase) recordUser(ctx context.Context, s entity.Subject, userID entity.ID, operation string, at time.Time,
	details map[string]any,
) error {
	return u.auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID,
		OccurredAt:     at,
		ActorKind:      entity.ActorUser,
		ActorID:        s.UserID,
		Operation:      operation,
		ObjectKind:     "user",
		ObjectID:       userID,
		Outcome:        entity.AuditSucceeded,
		CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		Details:        details,
	})
}

func (u *UseCase) roleSet(ctx context.Context, orgID, userID entity.ID) (entity.RoleSet, error) {
	version, err := u.orgs.RoleSetVersion(ctx, orgID, userID)
	if err != nil {
		return entity.RoleSet{}, err
	}
	roles, err := u.orgs.ActiveRoles(ctx, orgID, userID)
	if err != nil {
		return entity.RoleSet{}, err
	}
	return entity.RoleSet{Version: version, Roles: roles}, nil
}

// validateRoles applies the rules of org.role_assignment and reports every error at once.
func (u *UseCase) validateRoles(ctx context.Context, orgID entity.ID, roles []entity.RoleAssignment) error {
	verr := &entity.ValidationError{}
	if len(roles) > entity.MaxRoles {
		verr.Add("/data", entity.FieldOutOfRange, fmt.Sprintf("at most %d roles", entity.MaxRoles))
	}
	refs := map[entity.AreaKind][]entity.ID{}
	seen := make(map[string]bool, len(roles))
	for i, r := range roles {
		at := fmt.Sprintf("/data/%d", i)
		areas := 0
		for _, kind := range _areaKinds {
			if id := areaOf(r, kind); !id.IsZero() {
				areas++
				refs[kind] = append(refs[kind], id)
			}
		}
		switch {
		case !_roles[r.Role]:
			verr.Add(at+"/role", entity.FieldNotAllowedValue, "unknown role")
		case areas > 1:
			verr.Add(at, entity.FieldNotAllowedValue, "a role has at most one of team, project and service")
		case r.Role == entity.RoleAdministrator && areas > 0:
			verr.Add(at, entity.FieldNotAllowedValue, "administrator is assigned to the whole organization")
		case r.Role == entity.RoleProcessOwner && r.TeamID.IsZero():
			verr.Add(at+"/team_id", entity.FieldRequired, "process_owner is assigned to a team")
		case r.Role == entity.RoleServiceOwner && r.ServiceID.IsZero():
			verr.Add(at+"/service_id", entity.FieldRequired, "service_owner is assigned to a service")
		}
		if k := roleKey(r); seen[k] {
			verr.Add(at, entity.FieldDuplicate, "the role is listed twice for the same area")
		} else {
			seen[k] = true
		}
	}
	for _, kind := range _areaKinds {
		ids := refs[kind]
		if len(ids) == 0 {
			continue
		}
		found, err := u.orgs.ExistingAreas(ctx, orgID, kind, ids)
		if err != nil {
			return err
		}
		known := make(map[entity.ID]bool, len(found))
		for _, id := range found {
			known[id] = true
		}
		for i, r := range roles {
			if id := areaOf(r, kind); !id.IsZero() && !known[id] {
				verr.Add(fmt.Sprintf("/data/%d/%s_id", i, kind), entity.FieldUnknownReference, string(kind)+" is not found")
			}
		}
	}
	if len(verr.Fields) > 0 {
		return verr
	}
	return nil
}

func diffRoles(current, wanted []entity.RoleAssignment) (revoked, assigned []entity.RoleAssignment) {
	want := make(map[string]bool, len(wanted))
	for _, r := range wanted {
		want[roleKey(r)] = true
	}
	have := make(map[string]bool, len(current))
	for _, r := range current {
		have[roleKey(r)] = true
		if !want[roleKey(r)] {
			revoked = append(revoked, r)
		}
	}
	for _, r := range wanted {
		if !have[roleKey(r)] {
			assigned = append(assigned, r)
		}
	}
	return revoked, assigned
}

// roleChange keeps the previous and the new set when they fit the audit record, otherwise their sizes.
func roleChange(before, after []entity.RoleAssignment) map[string]any {
	keys := func(rs []entity.RoleAssignment) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = roleKey(r)
		}
		return out
	}
	d := map[string]any{"before": keys(before), "after": keys(after)}
	if b, err := json.Marshal(d); err == nil && len(b) <= entity.AuditDetailsLimit {
		return d
	}
	return map[string]any{"before_count": len(before), "after_count": len(after)}
}

// roleKey names an assignment: role@team:<id>, role@project:<id>, role@service:<id> or the role alone.
func roleKey(r entity.RoleAssignment) string {
	for _, kind := range _areaKinds {
		if id := areaOf(r, kind); !id.IsZero() {
			return string(r.Role) + "@" + string(kind) + ":" + id.String()
		}
	}
	return string(r.Role)
}

func areaOf(r entity.RoleAssignment, kind entity.AreaKind) entity.ID {
	switch kind {
	case entity.AreaTeam:
		return r.TeamID
	case entity.AreaProject:
		return r.ProjectID
	case entity.AreaService:
		return r.ServiceID
	}
	return entity.ID{}
}
