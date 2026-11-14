package org

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _operationReplaceStructure = "organization.structure.replace"

var _areaKinds = []entity.AreaKind{entity.AreaTeam, entity.AreaProject, entity.AreaService}

// structurePlan is the change a confirmation makes; an empty plan leaves the version as it is.
type structurePlan struct {
	created map[entity.AreaKind][]entity.Area
	updated map[entity.AreaKind][]entity.Area
	closed  map[entity.LinkKind][]entity.Link
	opened  map[entity.LinkKind][]entity.Link
}

func (p structurePlan) empty() bool {
	for _, m := range []int{len(p.created), len(p.updated), len(p.closed), len(p.opened)} {
		if m > 0 {
			return false
		}
	}
	return true
}

// Structure reads the teams, projects, services and the links in force now (UC-02).
func (u *UseCase) Structure(ctx context.Context, s entity.Subject) (entity.Structure, error) {
	var out entity.Structure
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		v, err := u.orgs.StructureVersion(ctx, s.OrganizationID)
		if err != nil {
			return err
		}
		out, err = u.read(ctx, s.OrganizationID, v, u.clock.Now())
		return err
	})
	return out, err
}

// ReplaceStructure replaces the whole structure confirmed by an administrator who saw baseVersion
// (FR-008, FR-012). The change, the new version and its audit record commit together; a body equal
// to the current structure changes nothing.
func (u *UseCase) ReplaceStructure(ctx context.Context, s entity.Subject, baseVersion int64, in entity.StructureInput) (entity.Structure, error) {
	orgID := s.OrganizationID
	known, err := u.knownAreas(ctx, orgID, in.ProjectAreas)
	if err != nil {
		return entity.Structure{}, err
	}
	var out entity.Structure
	err = u.tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		version, err := u.orgs.LockStructureVersion(ctx, orgID)
		if err != nil {
			return err
		}
		if version != baseVersion {
			return entity.ErrVersionConflict
		}
		now := u.clock.Now()
		current, err := u.read(ctx, orgID, version, now)
		if err != nil {
			return err
		}
		plan, err := u.plan(ctx, orgID, current, in, known)
		if err != nil {
			return err
		}
		if plan.empty() {
			out = current
			return nil
		}
		if err := u.apply(ctx, orgID, plan, now); err != nil {
			return err
		}
		if err := u.orgs.SetStructureVersion(ctx, orgID, version+1); err != nil {
			return err
		}
		err = u.auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: orgID,
			OccurredAt:     now,
			ActorKind:      entity.ActorUser,
			ActorID:        s.UserID,
			Operation:      _operationReplaceStructure,
			ObjectKind:     "organization",
			ObjectID:       orgID,
			Outcome:        entity.AuditSucceeded,
			BaseVersion:    strconv.FormatInt(version, 10),
			NewVersion:     strconv.FormatInt(version+1, 10),
			CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		})
		if err != nil {
			return err
		}
		out, err = u.read(ctx, orgID, version+1, now)
		return err
	})
	return out, err
}

// ListAreas pages the teams, projects or services of the scope by name, then by ID.
func (u *UseCase) ListAreas(ctx context.Context, s entity.Subject, scope entity.Scope, kind entity.AreaKind,
	f entity.AreaFilter, p entity.PageRequest,
) (entity.Page, error) {
	var page entity.Page
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.orgs.PageAreas(ctx, s.OrganizationID, kind, scope, f, p, u.clock.Now())
		if err != nil {
			return err
		}
		if len(items) > p.Limit {
			items = items[:p.Limit]
			last := items[len(items)-1]
			page.Next = &entity.PageKey{Name: last.Name, ID: last.ID}
		}
		page.Items = items
		return nil
	})
	return page, err
}

func (u *UseCase) read(ctx context.Context, orgID entity.ID, version int64, at time.Time) (entity.Structure, error) {
	out := entity.Structure{Version: version}
	var err error
	if out.Teams, err = u.orgs.AllAreas(ctx, orgID, entity.AreaTeam); err != nil {
		return out, err
	}
	if out.Projects, err = u.orgs.AllAreas(ctx, orgID, entity.AreaProject); err != nil {
		return out, err
	}
	if out.Services, err = u.orgs.AllAreas(ctx, orgID, entity.AreaService); err != nil {
		return out, err
	}
	if out.TeamProjects, err = u.orgs.CurrentLinks(ctx, orgID, entity.LinkTeamProject, at); err != nil {
		return out, err
	}
	if out.TeamMembers, err = u.orgs.CurrentLinks(ctx, orgID, entity.LinkTeamMember, at); err != nil {
		return out, err
	}
	out.ProjectAreas, err = u.orgs.CurrentLinks(ctx, orgID, entity.LinkProjectArea, at)
	return out, err
}

// ProjectAreas are the source areas of projects in force at the moment; it runs in the caller's organization
// transaction, as access reads it together with the profile.
func (u *UseCase) ProjectAreas(ctx context.Context, orgID entity.ID, at time.Time) ([]entity.Link, error) {
	return u.orgs.CurrentLinks(ctx, orgID, entity.LinkProjectArea, at)
}

// knownAreas are the areas of the body some connection of the organization collected; a link may name only
// such an area (FR-012).
func (u *UseCase) knownAreas(ctx context.Context, orgID entity.ID, links []entity.LinkInput) (map[entity.ID]bool, error) {
	if len(links) == 0 {
		return nil, nil
	}
	ids := make([]entity.ID, len(links))
	for i, l := range links {
		ids[i] = l.To
	}
	sources, err := u.sources.AreaSources(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	known := make(map[entity.ID]bool, len(sources))
	for _, src := range sources {
		known[src.AreaID] = true
	}
	return known, nil
}

// plan checks the whole body against the current structure and reports every field error at once.
func (u *UseCase) plan(ctx context.Context, orgID entity.ID, cur entity.Structure, in entity.StructureInput,
	knownAreas map[entity.ID]bool,
) (structurePlan, error) {
	p := structurePlan{
		created: map[entity.AreaKind][]entity.Area{},
		updated: map[entity.AreaKind][]entity.Area{},
		closed:  map[entity.LinkKind][]entity.Link{},
		opened:  map[entity.LinkKind][]entity.Link{},
	}
	verr := &entity.ValidationError{}
	given := map[entity.AreaKind]map[entity.ID]bool{}
	for _, kind := range _areaKinds {
		ids, err := u.planAreas(p, verr, kind, currentAreas(cur, kind), inputAreas(in, kind))
		if err != nil {
			return p, err
		}
		given[kind] = ids
	}

	persons, err := u.persons(ctx, orgID, in.TeamMembers)
	if err != nil {
		return p, err
	}
	planLinks(p, verr, entity.LinkTeamProject, cur.TeamProjects, in.TeamProjects, given[entity.AreaTeam], given[entity.AreaProject])
	planLinks(p, verr, entity.LinkTeamMember, cur.TeamMembers, in.TeamMembers, given[entity.AreaTeam], persons)
	planLinks(p, verr, entity.LinkProjectArea, cur.ProjectAreas, in.ProjectAreas, given[entity.AreaProject], knownAreas)
	for i := range in.Components {
		verr.Add(fmt.Sprintf("/components/%d/repository_area_id", i), entity.FieldUnknownReference, "repository is not known from connections")
	}
	if len(verr.Fields) > 0 {
		return p, verr
	}
	return p, nil
}

// planAreas returns the IDs the body keeps; existing records may only change name and status.
func (u *UseCase) planAreas(p structurePlan, verr *entity.ValidationError, kind entity.AreaKind, cur []entity.Area,
	in []entity.AreaInput,
) (map[entity.ID]bool, error) {
	field := "/" + string(kind) + "s"
	if limit := maxAreas(kind); len(in) > limit {
		verr.Add(field, entity.FieldOutOfRange, fmt.Sprintf("at most %d items", limit))
	}
	existing := make(map[entity.ID]entity.Area, len(cur))
	for _, a := range cur {
		existing[a.ID] = a
	}
	kept := make(map[entity.ID]bool, len(in))
	for i, a := range in {
		at := fmt.Sprintf("%s/%d", field, i)
		validArea(verr, at, a)
		if a.ID.IsZero() {
			id, err := u.newID()
			if err != nil {
				return nil, fmt.Errorf("org: new %s ID: %w", kind, err)
			}
			a.ID = id
			p.created[kind] = append(p.created[kind], entity.Area(a))
			continue
		}
		old, ok := existing[a.ID]
		switch {
		case !ok:
			verr.Add(at+"/id", entity.FieldUnknownReference, string(kind)+" is not found")
			continue
		case kept[a.ID]:
			verr.Add(at+"/id", entity.FieldDuplicate, string(kind)+" is listed twice")
			continue
		}
		kept[a.ID] = true
		if old.Name != a.Name || old.Status != a.Status {
			p.updated[kind] = append(p.updated[kind], entity.Area(a))
		}
	}
	for _, a := range cur {
		if !kept[a.ID] {
			verr.Add(field, entity.FieldRequired, fmt.Sprintf("%s %s is missing; archive it with status archived", kind, a.ID))
		}
	}
	return kept, nil
}

func validArea(verr *entity.ValidationError, at string, a entity.AreaInput) {
	switch {
	case strings.TrimSpace(a.Name) == "":
		verr.Add(at+"/name", entity.FieldRequired, "name is empty")
	case utf8.RuneCountInString(a.Name) > entity.MaxAreaName:
		verr.Add(at+"/name", entity.FieldTooLong, fmt.Sprintf("at most %d characters", entity.MaxAreaName))
	}
	if a.Status != "active" && a.Status != "archived" {
		verr.Add(at+"/status", entity.FieldNotAllowedValue, "status is active or archived")
	}
}

// planLinks closes the current links missing from the body and opens the new ones from now (M0).
func planLinks(p structurePlan, verr *entity.ValidationError, kind entity.LinkKind, cur []entity.Link, in []entity.LinkInput,
	froms, tos map[entity.ID]bool,
) {
	field := "/" + string(kind) + "s"
	if len(in) > entity.MaxLinks {
		verr.Add(field, entity.FieldOutOfRange, fmt.Sprintf("at most %d items", entity.MaxLinks))
	}
	fromField, toField := linkFields(kind)
	type pair struct{ from, to entity.ID }
	wanted := make(map[pair]bool, len(in))
	for i, l := range in {
		at := fmt.Sprintf("%s/%d", field, i)
		if l.ValidFrom != nil {
			verr.Add(at+"/valid_from", entity.FieldNotAllowedValue, "dates of links are set from M1; send null")
		}
		if l.ValidTo != nil {
			verr.Add(at+"/valid_to", entity.FieldNotAllowedValue, "dates of links are set from M1; send null")
		}
		if !froms[l.From] {
			verr.Add(at+fromField, entity.FieldUnknownReference, "the record is not in the structure")
		}
		if !tos[l.To] {
			verr.Add(at+toField, entity.FieldUnknownReference, "the linked record is not in the structure or not known from connections")
		}
		k := pair{l.From, l.To}
		if wanted[k] {
			verr.Add(at, entity.FieldDuplicate, "interval overlaps another one of the same pair")
		}
		wanted[k] = true
	}
	have := make(map[pair]bool, len(cur))
	for _, l := range cur {
		k := pair{l.From, l.To}
		have[k] = true
		if !wanted[k] {
			p.closed[kind] = append(p.closed[kind], l)
		}
	}
	for _, l := range in {
		k := pair{l.From, l.To}
		if !have[k] {
			have[k] = true
			p.opened[kind] = append(p.opened[kind], entity.Link{From: l.From, To: l.To})
		}
	}
}

func (u *UseCase) apply(ctx context.Context, orgID entity.ID, p structurePlan, now time.Time) error {
	for _, kind := range _areaKinds {
		for _, a := range p.created[kind] {
			if err := u.orgs.CreateArea(ctx, orgID, kind, a, now); err != nil {
				return err
			}
		}
		for _, a := range p.updated[kind] {
			if err := u.orgs.UpdateArea(ctx, orgID, kind, a); err != nil {
				return err
			}
		}
	}
	seq, err := u.tenants.NextSeq(ctx, orgID)
	if err != nil {
		return err
	}
	for _, kind := range []entity.LinkKind{entity.LinkTeamProject, entity.LinkTeamMember, entity.LinkProjectArea} {
		for _, l := range p.closed[kind] {
			if err := u.closeLink(ctx, orgID, kind, l, seq, now); err != nil {
				return err
			}
		}
		for _, l := range p.opened[kind] {
			l.ValidFrom = now
			if err := u.orgs.InsertLink(ctx, orgID, kind, l, seq); err != nil {
				return overlap(kind, err)
			}
		}
	}
	return nil
}

// closeLink supersedes the version and records the interval ending now; a link opened at this very
// moment never was in force, so nothing replaces it.
func (u *UseCase) closeLink(ctx context.Context, orgID entity.ID, kind entity.LinkKind, l entity.Link, seq int64, now time.Time) error {
	if err := u.orgs.SupersedeLink(ctx, orgID, kind, l.RowID, seq, now); err != nil {
		return err
	}
	if !l.ValidFrom.Before(now) {
		return nil
	}
	l.ValidTo = now
	return overlap(kind, u.orgs.InsertLink(ctx, orgID, kind, l, seq))
}

func linkFields(kind entity.LinkKind) (from, to string) {
	switch kind {
	case entity.LinkTeamMember:
		return "/team_id", "/person_id"
	case entity.LinkProjectArea:
		return "/project_id", "/area_id"
	case entity.LinkTeamProject:
	}
	return "/team_id", "/project_id"
}

func overlap(kind entity.LinkKind, err error) error {
	if errors.Is(err, entity.ErrIntervalOverlap) {
		return &entity.ValidationError{Fields: []entity.FieldError{{
			Field: "/" + string(kind) + "s", Code: entity.FieldDuplicate, Detail: "interval overlaps another one of the same pair",
		}}}
	}
	return err
}

func (u *UseCase) persons(ctx context.Context, orgID entity.ID, links []entity.LinkInput) (map[entity.ID]bool, error) {
	ids := make([]entity.ID, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.To)
	}
	found, err := u.orgs.ExistingPersons(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[entity.ID]bool, len(found))
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

func currentAreas(s entity.Structure, kind entity.AreaKind) []entity.Area {
	switch kind {
	case entity.AreaTeam:
		return s.Teams
	case entity.AreaProject:
		return s.Projects
	case entity.AreaService:
		return s.Services
	}
	return nil
}

func inputAreas(in entity.StructureInput, kind entity.AreaKind) []entity.AreaInput {
	switch kind {
	case entity.AreaTeam:
		return in.Teams
	case entity.AreaProject:
		return in.Projects
	case entity.AreaService:
		return in.Services
	}
	return nil
}

func maxAreas(kind entity.AreaKind) int {
	if kind == entity.AreaTeam {
		return entity.MaxTeams
	}
	return entity.MaxProjects
}
