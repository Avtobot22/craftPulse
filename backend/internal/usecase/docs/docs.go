package docs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// References is org: a service and its explicitly selected people must belong to the organization.
type References interface {
	CheckDocumentationReferences(ctx context.Context, orgID, serviceID entity.ID, persons []entity.ID) error
}

// Areas is normalize: only collected repository areas may be Git documentation roots.
type Areas interface {
	RepositoryAreas(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ID, error)
}

type Auditor interface {
	Record(ctx context.Context, e entity.AuditEvent) error
}

type Deps struct {
	Repo       repo.DocumentationRepo
	Tenants    repo.TenantRepo
	Tx         repo.Transactor
	Access     usecase.Access
	References References
	Areas      Areas
	Documents  Documents
	Auditor    Auditor
	Clock      clock.Clock
	NewID      func() (entity.ID, error)
}

type UseCase struct{ d Deps }

func New(d Deps) *UseCase { return &UseCase{d: d} }

func (u *UseCase) ListRoots(ctx context.Context, s entity.Subject, scope entity.Scope, serviceID entity.ID) (entity.DocumentationRootSet, error) {
	if !slices.Contains(scope.Services(), serviceID) {
		return entity.DocumentationRootSet{}, entity.ErrNotFound
	}
	var out entity.DocumentationRootSet
	err := u.d.Tx.JoinOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		if err := u.d.References.CheckDocumentationReferences(ctx, s.OrganizationID, serviceID, nil); err != nil {
			return err
		}
		var err error
		out, err = u.latest(ctx, s.OrganizationID, serviceID)
		return err
	})
	return out, err
}

func (u *UseCase) latest(ctx context.Context, orgID, serviceID entity.ID) (entity.DocumentationRootSet, error) {
	out, err := u.d.Repo.LatestRootSet(ctx, orgID, serviceID)
	if errors.Is(err, entity.ErrNotFound) {
		return entity.DocumentationRootSet{ServiceID: serviceID, Roots: []entity.DocumentationRoot{}}, nil
	}
	if err != nil {
		return entity.DocumentationRootSet{}, err
	}
	out.Roots, err = u.d.Repo.RootsAt(ctx, orgID, serviceID, out.KnownSeq)
	return out, err
}

func (u *UseCase) ReplaceRoots(ctx context.Context, s entity.Subject, serviceID entity.ID, base int64, key entity.ID,
	roots []entity.DocumentationRoot,
) (entity.DocumentationRootSet, error) {
	scope, err := u.d.Access.Authorize(ctx, s, entity.AccessRequest{Operation: "replaceDocumentationRoots", Action: "docs.manage", Area: entity.AreaRef{Kind: entity.AreaService, ID: serviceID}})
	if err != nil {
		return entity.DocumentationRootSet{}, err
	}
	roots, digest, err := validate(roots)
	if err != nil {
		return entity.DocumentationRootSet{}, err
	}
	areas, persons := []entity.ID{}, []entity.ID{}
	for i := range roots {
		r := &roots[i]
		areas = append(areas, r.AreaID)
		if r.OwnerPersonID != nil {
			persons = append(persons, *r.OwnerPersonID)
		}
	}
	// The service owner confirming this set is the subject of the source-rights decision.
	allowed, err := u.d.Access.Decide(ctx, s, scope, entity.SourceQuery{Operation: "replaceDocumentationRoots", Areas: areas})
	if err != nil {
		return entity.DocumentationRootSet{}, err
	}
	for i := range roots {
		r := &roots[i]
		if !allowed.Area(r.AreaID) {
			return entity.DocumentationRootSet{}, invalid(i, "area_id", entity.FieldOutsideScope)
		}
	}
	var out entity.DocumentationRootSet
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		if err := u.d.Repo.LockRootWrites(ctx, s.OrganizationID); err != nil {
			return err
		}
		saved, actor, err := u.d.Repo.RootSetByConfirmation(ctx, s.OrganizationID, key)
		if err == nil {
			if saved.ServiceID != serviceID || actor != s.UserID || !bytes.Equal(saved.ContentHash, digest) {
				return entity.ErrIdempotencyKeyReused
			}
			saved.Roots, err = u.d.Repo.RootsAt(ctx, s.OrganizationID, serviceID, saved.KnownSeq)
			out = saved
			return err
		}
		if !errors.Is(err, entity.ErrNotFound) {
			return err
		}
		current, err := u.latest(ctx, s.OrganizationID, serviceID)
		if err != nil {
			return err
		}
		if current.Version != base {
			return entity.ErrVersionConflict
		}
		if bytes.Equal(current.ContentHash, digest) {
			out = current
			return u.d.Repo.RememberRootConfirmation(ctx, s.OrganizationID, key, s.UserID, out, u.d.Clock.Now())
		}
		if err := u.d.References.CheckDocumentationReferences(ctx, s.OrganizationID, serviceID, persons); err != nil {
			return err
		}
		repositories, err := u.d.Areas.RepositoryAreas(ctx, s.OrganizationID, areas)
		if err != nil {
			return err
		}
		for i := range roots {
			r := &roots[i]
			if !slices.Contains(repositories, r.AreaID) {
				return invalid(i, "area_id", entity.FieldOutsideScope)
			}
		}
		seq, err := u.d.Tenants.NextSeq(ctx, s.OrganizationID)
		if err != nil {
			return err
		}
		slices.SortFunc(roots, func(a, b entity.DocumentationRoot) int {
			if n := slices.Compare(a.AreaID[:], b.AreaID[:]); n != 0 {
				return n
			}
			return strings.Compare(a.PathPrefix, b.PathPrefix)
		})
		out = entity.DocumentationRootSet{
			ServiceID: serviceID, Version: current.Version + 1, KnownSeq: seq, ContentHash: digest,
			Confirmation: entity.Confirmation{ConfirmedBy: s.UserID, ConfirmedAt: u.d.Clock.Now(), ConfirmationID: key}, Roots: roots,
		}
		for i := range out.Roots {
			id, err := u.d.NewID()
			if err != nil {
				return fmt.Errorf("docs: new root id: %w", err)
			}
			out.Roots[i].ID, out.Roots[i].ServiceID = id, serviceID
			out.Roots[i].Knowledge = entity.Knowledge{KnownSeq: seq}
			out.Roots[i].Confirmation = out.Confirmation
		}
		if err := u.d.Repo.SupersedeRoots(ctx, s.OrganizationID, serviceID, seq, out.ConfirmedAt); err != nil {
			return err
		}
		if err := u.d.Repo.InsertRootSet(ctx, s.OrganizationID, out); err != nil {
			return err
		}
		if err := u.d.Repo.RememberRootConfirmation(ctx, s.OrganizationID, key, s.UserID, out, out.ConfirmedAt); err != nil {
			return err
		}
		return u.d.Auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: s.OrganizationID, ActorKind: entity.ActorUser, ActorID: s.UserID,
			OccurredAt: out.ConfirmedAt, Operation: "docs.roots.replace", ObjectKind: "documentation_roots", ObjectID: serviceID, AreaID: serviceID,
			Outcome: entity.AuditSucceeded, BaseVersion: strconv.FormatInt(current.Version, 10), NewVersion: strconv.FormatInt(out.Version, 10),
			CorrelationID: entity.ID(logger.CorrelationID(ctx)), Details: map[string]any{"confirmation_id": key.String()},
		})
	})
	if err != nil {
		return entity.DocumentationRootSet{}, err
	}
	return out, nil
}

// RootsAt reads roots in a knowledge slice, constrained to the service and source area access allowed.
func (u *UseCase) RootsAt(ctx context.Context, orgID entity.ID, scope entity.Scope, serviceID, areaID entity.ID, seq int64) ([]entity.DocumentationRoot, error) {
	if !slices.Contains(scope.Services(), serviceID) || !scope.Area(areaID) {
		return nil, entity.ErrNotFound
	}
	var roots []entity.DocumentationRoot
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		roots, err = u.d.Repo.RootsAt(ctx, orgID, serviceID, seq)
		return err
	})
	return slices.DeleteFunc(roots, func(r entity.DocumentationRoot) bool { return r.AreaID != areaID }), err
}

func validate(in []entity.DocumentationRoot) ([]entity.DocumentationRoot, []byte, error) {
	if len(in) > 100 {
		return nil, nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/data", Code: entity.FieldOutOfRange}}}
	}
	roots := slices.Clone(in)
	for i := range roots {
		r := &roots[i]
		if r.AreaID.IsZero() {
			return nil, nil, invalid(i, "area_id", entity.FieldRequired)
		}
		if r.ProfileID != "git-markdown-openapi" {
			return nil, nil, invalid(i, "profile_id", entity.FieldNotAllowedValue)
		}
		if r.Audience != "team" && r.Audience != "organization" && r.Audience != "external" {
			return nil, nil, invalid(i, "audience", entity.FieldNotAllowedValue)
		}
		p := r.PathPrefix
		if len(p) > 1024 || strings.ContainsAny(p, "\\\x00") || (p != "" && (path.Clean(p) != p || path.IsAbs(p) || p == "." || p == ".." || strings.HasPrefix(p, "../"))) {
			return nil, nil, invalid(i, "path_prefix", entity.FieldInvalidFormat)
		}
		for j := range roots[:i] {
			earlier := &roots[j]
			if earlier.AreaID == r.AreaID && earlier.PathPrefix == p {
				return nil, nil, invalid(i, "path_prefix", entity.FieldDuplicate)
			}
		}
	}
	// Hash only the input: generated IDs and knowledge metadata are not part of a confirmation.
	type input struct {
		AreaID   entity.ID  `json:"area_id"`
		Path     string     `json:"path_prefix"`
		Profile  string     `json:"profile_id"`
		Owner    *entity.ID `json:"owner_person_id"`
		Audience string     `json:"audience"`
	}
	form := make([]input, len(roots))
	for i := range roots {
		r := &roots[i]
		form[i] = input{r.AreaID, r.PathPrefix, r.ProfileID, r.OwnerPersonID, r.Audience}
	}
	slices.SortFunc(form, func(a, b input) int {
		if n := slices.Compare(a.AreaID[:], b.AreaID[:]); n != 0 {
			return n
		}
		return strings.Compare(a.Path, b.Path)
	})
	encoded, err := json.Marshal(form)
	if err != nil {
		return nil, nil, fmt.Errorf("docs: encode roots: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return roots, digest[:], nil
}

func invalid(i int, field string, code string) error {
	return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/data/" + strconv.Itoa(i) + "/" + field, Code: code}}}
}

// AreaRoots are the confirmed roots in force in the repositories, for the profile of the step of CI of a project;
// the caller decides which project the step may read.
func (u *UseCase) AreaRoots(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.DocumentationRoot, error) {
	if len(areaIDs) == 0 {
		return nil, nil
	}
	var roots []entity.DocumentationRoot
	err := u.d.Tx.JoinOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		roots, err = u.d.Repo.CurrentAreaRoots(ctx, orgID, areaIDs)
		return err
	})
	return roots, err
}
