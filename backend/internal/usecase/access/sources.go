package access

import (
	"context"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// closure is why an area or an object is closed; open is the zero value.
type closure int

const (
	open closure = iota
	// outside is an area no project of the roles holds: it looks like one that does not exist.
	outside
	denied
	unavailable
	lost
)

type gate struct {
	closure    closure
	connection entity.ID
}

// Decide narrows the scope of Authorize by the rights of the user in the sources, checked now or at most
// 60 s ago (TZ 7.3, 13.4, 13.5). An area is open when a project of the roles holds it and the source allows
// it; a denied, unknown or failed check, a paused connection, a lost access and a missing account close it.
// An area no connection collected came only from an import and has no source rights to check. An object with
// its own restriction is checked by itself and needs its area open as well. Closed data adds a note without
// the number and names of what is hidden; a refusal of a requested area or object is audited. The result keeps
// what the given scope already allows, so a read first opens its areas and then the objects found in them.
func (u *UseCase) Decide(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.SourceQuery) (entity.Scope, error) {
	if !s.UploadTokenID.IsZero() {
		return scope, nil
	}
	org := s.OrganizationID
	var links []entity.Link
	err := u.tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		links, err = u.structure.ProjectAreas(ctx, org, u.clock.Now())
		return err
	})
	if err != nil {
		return entity.Scope{}, err
	}
	areasOf, owners := map[entity.ID][]entity.ID{}, map[entity.ID][]entity.ID{}
	for _, l := range links {
		areasOf[l.From] = append(areasOf[l.From], l.To)
		owners[l.To] = append(owners[l.To], l.From)
	}
	roleProjects := scope.Projects()
	inRole := func(area entity.ID) bool {
		return scope.Whole() || slices.ContainsFunc(owners[area], func(p entity.ID) bool { return slices.Contains(roleProjects, p) })
	}

	gates := map[entity.ID]gate{}
	var check []entity.ID
	want := func(area entity.ID) {
		if _, seen := gates[area]; seen || slices.Contains(check, area) {
			return
		}
		if !inRole(area) {
			gates[area] = gate{closure: outside}
			return
		}
		check = append(check, area)
	}
	if q.Projects {
		for _, p := range roleProjects {
			for _, a := range areasOf[p] {
				want(a)
			}
		}
	}
	for _, a := range q.Areas {
		want(a)
	}
	for _, o := range q.Objects {
		want(o.AreaID)
	}
	targets, err := u.areaGates(ctx, s, check, gates)
	if err != nil {
		return entity.Scope{}, err
	}
	objects := u.objectGates(ctx, s, q.Objects, gates, targets)
	return u.grant(ctx, s, scope, q, areasOf, gates, objects)
}

// areaGates decides every area to check and returns the targets of the open ones, which their objects are
// checked with.
func (u *UseCase) areaGates(ctx context.Context, s entity.Subject, areas []entity.ID, gates map[entity.ID]gate) (map[entity.ID]target, error) {
	if len(areas) == 0 {
		return nil, nil
	}
	org := s.OrganizationID
	sources, err := u.sources.AreaSources(ctx, org, areas)
	if err != nil {
		return nil, err
	}
	accounts := map[entity.ID]*entity.UserAccount{}
	var pending []target
	for _, a := range areas {
		src, g, ok := pick(sources, a)
		if !ok {
			gates[a] = g
			continue
		}
		acc, cached := accounts[src.InstanceID]
		if !cached {
			found, ok, err := u.accounts.UserSourceAccount(ctx, org, s.UserID, src.InstanceID)
			if err != nil {
				return nil, err
			}
			if ok {
				acc = &found
			}
			accounts[src.InstanceID] = acc
		}
		if acc == nil {
			gates[a] = gate{closure: denied}
			continue
		}
		pending = append(pending, target{id: a, source: src, account: *acc})
	}
	opened := map[entity.ID]target{}
	for i, d := range u.run(ctx, org, s.UserID, pending) {
		t := pending[i]
		gates[t.id] = verdictGate(d, t.source.ConnectionID)
		if gates[t.id].closure == open {
			opened[t.id] = t
		}
	}
	return opened, nil
}

// pick finds the scope that collects the area now; false with the gate when the area is decided without a
// check: no connection collected it (an import), or its connection is gone, lost access or is not active.
func pick(sources []entity.AreaSource, area entity.ID) (entity.AreaSource, gate, bool) {
	var newest *entity.AreaSource
	for i := range sources {
		src := &sources[i]
		if src.AreaID != area {
			continue
		}
		if newest == nil {
			newest = src
		}
		if src.Active && src.ConnectionStatus != entity.ConnectionDeleted {
			newest = src
			break
		}
	}
	switch {
	case newest == nil:
		return entity.AreaSource{}, gate{}, false
	case !newest.Active, newest.ConnectionStatus == entity.ConnectionDeleted, newest.ConnectionStatus == entity.ConnectionAccessLost:
		return entity.AreaSource{}, gate{closure: lost, connection: newest.ConnectionID}, false
	case newest.ConnectionStatus != entity.ConnectionActive:
		return entity.AreaSource{}, gate{closure: unavailable, connection: newest.ConnectionID}, false
	}
	return *newest, gate{}, true
}

func (u *UseCase) objectGates(ctx context.Context, s entity.Subject, objects []entity.RestrictedObject, areas map[entity.ID]gate,
	targets map[entity.ID]target,
) map[entity.ID]gate {
	out := make(map[entity.ID]gate, len(objects))
	var pending []target
	for i := range objects {
		o := &objects[i]
		if g := areas[o.AreaID]; g.closure != open {
			out[o.ID] = g
			continue
		}
		t, ok := targets[o.AreaID]
		if !ok {
			// The area came from an import: the source has no rights of its objects to check.
			continue
		}
		pending = append(pending, target{id: o.ID, source: t.source, account: t.account, object: o})
	}
	for i, d := range u.run(ctx, s.OrganizationID, s.UserID, pending) {
		out[pending[i].id] = verdictGate(d, pending[i].source.ConnectionID)
	}
	return out
}

// run checks the targets in parallel; each answer lands in its own slot.
func (u *UseCase) run(ctx context.Context, org, user entity.ID, targets []target) []string {
	out := make([]string, len(targets))
	var g errgroup.Group
	g.SetLimit(_parallelChecks)
	for i := range targets {
		g.Go(func() error {
			out[i] = u.checks.decide(ctx, org, user, targets[i])
			return nil
		})
	}
	_ = g.Wait() // explicitly ignored: a check reports unknown instead of an error
	return out
}

func verdictGate(decision string, connection entity.ID) gate {
	switch decision {
	case entity.SourceAllowed:
		return gate{}
	case entity.SourceDenied:
		return gate{closure: denied}
	}
	return gate{closure: unavailable, connection: connection}
}

// grant builds the narrowed scope. Listing projects turns a role over the whole organization into the
// explicit list of the projects it may see.
func (u *UseCase) grant(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.SourceQuery,
	areasOf map[entity.ID][]entity.ID, areas, objects map[entity.ID]gate,
) (entity.Scope, error) {
	g := entity.ScopeGrant{
		Whole: scope.Whole() && !q.Projects, Teams: scope.Teams(), Projects: scope.Projects(), Services: scope.Services(),
		Areas: scope.Areas(), Objects: scope.Objects(),
	}
	notes := scope.Notes()
	if q.Projects {
		g.Projects = g.Projects[:0]
		for _, p := range scope.Projects() {
			own := areasOf[p]
			if len(own) == 0 || slices.ContainsFunc(own, func(a entity.ID) bool { return areas[a].closure == open }) {
				g.Projects = append(g.Projects, p)
				continue
			}
			for _, a := range own {
				notes = note(notes, areas[a])
			}
		}
	}
	for id, gt := range areas {
		if gt.closure == open && !slices.Contains(g.Areas, id) {
			g.Areas = append(g.Areas, id)
		}
	}
	var refused []entity.ID
	reason := ""
	for _, a := range q.Areas {
		if gt := areas[a]; gt.closure != open {
			notes = note(notes, gt)
			refused = append(refused, a)
			reason = firstReason(reason, gt)
		}
	}
	for _, o := range q.Objects {
		gt, checked := objects[o.ID]
		if !checked || gt.closure == open {
			if !slices.Contains(g.Objects, o.ID) {
				g.Objects = append(g.Objects, o.ID)
			}
			continue
		}
		notes = note(notes, gt)
		refused = append(refused, o.ID)
		reason = firstReason(reason, gt)
	}
	slices.SortFunc(g.Areas, compareIDs)
	g.Notes = notes
	if len(refused) == 0 {
		return entity.GrantScope(g), nil
	}
	err := u.auditor.RecordSeparately(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID,
		ActorKind:      entity.ActorUser,
		ActorID:        s.UserID,
		Operation:      q.Operation,
		ObjectKind:     "source_data",
		ObjectID:       refused[0],
		Outcome:        entity.AuditDenied,
		ReasonCode:     reason,
		CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		Details:        map[string]any{"refused": refusedIDs(refused)},
	})
	if err != nil {
		return entity.Scope{}, err
	}
	return entity.GrantScope(g), nil
}

// note adds the mark of a closure once; an area outside the roles gives none, as it looks like a missing one.
func note(notes []entity.AccessNote, g gate) []entity.AccessNote {
	var n entity.AccessNote
	switch g.closure {
	case open, outside:
		return notes
	case denied:
		n = entity.AccessNote{Code: entity.NoteRestricted}
	case unavailable:
		n = entity.AccessNote{Code: entity.NoteCheckUnavailable, ConnectionID: g.connection}
	case lost:
		n = entity.AccessNote{Code: entity.NoteAccessLost, ConnectionID: g.connection}
	}
	if slices.Contains(notes, n) {
		return notes
	}
	return append(notes, n)
}

func firstReason(reason string, g gate) string {
	if reason != "" {
		return reason
	}
	switch g.closure {
	case outside:
		return ReasonOutsideScope
	case unavailable:
		return ReasonSourceUnavailable
	case lost:
		return ReasonSourceAccessLost
	case open, denied:
	}
	return ReasonSourceDenied
}

// _maxRefused bounds the IDs one audit record lists.
const _maxRefused = 100

func refusedIDs(ids []entity.ID) []string {
	out := make([]string, 0, min(len(ids), _maxRefused))
	for _, id := range ids[:min(len(ids), _maxRefused)] {
		out = append(out, id.String())
	}
	return out
}

func compareIDs(a, b entity.ID) int {
	return slices.Compare(a[:], b[:])
}
