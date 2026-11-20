package change

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_operationConfirm = "check_profile.confirm"
	_maxEntries       = 1000
)

// _required are the kinds every component of a supported language keeps: an external analyzer of the code does
// not turn off the checks of secrets and dependencies (FR-035 A1).
var _required = []string{entity.AnalysisSecrets, entity.AnalysisSCA}

// CheckProfile is the matrix of checks of a project in force with the components no profile covers; a project
// without a confirmed matrix is not found.
func (u *UseCase) CheckProfile(ctx context.Context, s entity.Subject, projectID entity.ID) (entity.CheckProfileVersion, error) {
	var out entity.CheckProfileVersion
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		if out, err = u.d.Repo.LatestProfileVersion(ctx, s.OrganizationID, projectID); err != nil {
			return err
		}
		components, err := u.d.Components.ProjectComponents(ctx, s.OrganizationID, projectID, u.d.Clock.Now())
		if err != nil {
			return err
		}
		out.Uncovered = u.uncovered(out.Entries, components)
		return nil
	})
	return out, err
}

// CreateCheckProfileVersion confirms a new matrix of a project by the rules of the versions of a mapping: a repeat
// of the key returns what it saved, a stale base is a conflict, a matrix equal to the current one creates nothing.
func (u *UseCase) CreateCheckProfileVersion(ctx context.Context, s entity.Subject, projectID entity.ID, baseVersion int, key entity.ID,
	d entity.CheckProfileDraft,
) (v entity.CheckProfileVersion, created bool, err error) {
	org := s.OrganizationID
	id, err := u.d.NewID()
	if err != nil {
		return entity.CheckProfileVersion{}, false, errorf("new profile version id: %w", err)
	}
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		created = false
		components, err := u.d.Components.ProjectComponents(ctx, org, projectID, u.d.Clock.Now())
		if err != nil {
			return err
		}
		entries, err := u.validate(d, components)
		if err != nil {
			return err
		}
		hash := contentHash(entries)
		saved, err := u.d.Repo.ProfileVersionByConfirmation(ctx, org, key)
		if err == nil {
			if saved.ProjectID != projectID || saved.ConfirmedBy != s.UserID || !bytes.Equal(saved.ContentHash, hash) {
				return entity.ErrIdempotencyKeyReused
			}
			v, created = saved, true
			v.Uncovered = u.uncovered(v.Entries, components)
			return nil
		}
		if !errors.Is(err, entity.ErrNotFound) {
			return err
		}
		current, err := u.d.Repo.LatestProfileVersion(ctx, org, projectID)
		latest := current.VersionNo
		if errors.Is(err, entity.ErrNotFound) {
			latest, err = 0, nil
		}
		if err != nil {
			return err
		}
		if latest != baseVersion {
			return entity.ErrVersionConflict
		}
		if latest > 0 && bytes.Equal(current.ContentHash, hash) {
			v = current
			v.Uncovered = u.uncovered(v.Entries, components)
			return nil
		}
		seq, err := u.d.Tenants.NextSeq(ctx, org)
		if err != nil {
			return err
		}
		v = entity.CheckProfileVersion{
			ID: id, ProjectID: projectID, VersionNo: latest + 1, ContentHash: hash, KnownSeq: seq, Entries: entries,
			Confirmation: entity.Confirmation{ConfirmedBy: s.UserID, ConfirmedAt: u.d.Clock.Now(), ConfirmationID: key},
		}
		if latest > 0 {
			base := latest
			v.BaseVersionNo = &base
		}
		if err := u.d.Repo.InsertProfileVersion(ctx, org, v); err != nil {
			return err
		}
		created = true
		v.Uncovered = u.uncovered(v.Entries, components)
		return u.d.Auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: org, OccurredAt: v.ConfirmedAt, ActorKind: entity.ActorUser, ActorID: s.UserID, Operation: _operationConfirm,
			ObjectKind: "check_profile_version", ObjectID: v.ID, AreaID: projectID, Outcome: entity.AuditSucceeded,
			BaseVersion: strconv.Itoa(latest), NewVersion: strconv.Itoa(v.VersionNo), CorrelationID: entity.ID(logger.CorrelationID(ctx)),
		})
	})
	if err != nil {
		return entity.CheckProfileVersion{}, false, err
	}
	return v, created, nil
}

// validate checks a draft against the components of the project and the catalog and gives the rows with the
// versions of the tools of their profiles; every error of the draft is reported at once.
func (u *UseCase) validate(d entity.CheckProfileDraft, components []entity.Component) ([]entity.ProfileEntry, error) {
	verr := &entity.ValidationError{}
	if len(d.Entries) > _maxEntries {
		verr.Add("/entries", entity.FieldOutOfRange, fmt.Sprintf("at most %d entries", _maxEntries))
	}
	known := make(map[entity.ID]bool, len(components))
	for i := range components {
		known[components[i].ID] = true
	}
	type key struct {
		component      entity.ID
		language, kind string
	}
	seen := map[key]bool{}
	kinds := map[entity.ID]map[string]bool{}
	supported := map[entity.ID]bool{}
	out := make([]entity.ProfileEntry, 0, len(d.Entries))
	for i, e := range d.Entries {
		at := "/entries/" + strconv.Itoa(i)
		if !known[e.ComponentID] {
			verr.Add(at+"/component_id", entity.FieldUnknownReference, "the component is not a confirmed component of the project")
		}
		p, ok := u.profiles[e.ProfileID]
		switch {
		case !slices.Contains(_kinds, e.AnalysisKind):
			verr.Add(at+"/analysis_kind", entity.FieldNotAllowedValue, "the analysis kind is unknown")
		case !ok || p.Mode != e.Mode:
			verr.Add(at+"/profile_id", entity.FieldNotAllowedValue, "no profile of the catalog with this mode")
		case !slices.Contains(p.AnalysisKinds, e.AnalysisKind):
			verr.Add(at+"/analysis_kind", entity.FieldNotAllowedValue, "the profile does not support the analysis kind")
		case !languageOf(&p, e.Language):
			verr.Add(at+"/language", entity.FieldNotAllowedValue, "the profile does not support the language")
		}
		k := key{e.ComponentID, e.Language, e.AnalysisKind}
		if seen[k] {
			verr.Add(at, entity.FieldDuplicate, "one row per component, language and analysis kind")
		}
		seen[k] = true
		if kinds[e.ComponentID] == nil {
			kinds[e.ComponentID] = map[string]bool{}
		}
		kinds[e.ComponentID][e.AnalysisKind] = true
		if e.Language != "" && u.builtinLanguage(e.Language) {
			supported[e.ComponentID] = true
		}
		out = append(out, entity.ProfileEntry{
			ComponentID: e.ComponentID, Language: e.Language, AnalysisKind: e.AnalysisKind, Mode: e.Mode, ProfileID: e.ProfileID,
			ToolVersions: maps.Clone(p.ToolVersions), RulesHash: p.RulesHash,
		})
		if out[len(out)-1].ToolVersions == nil {
			out[len(out)-1].ToolVersions = map[string]string{}
		}
	}
	for _, c := range slices.SortedFunc(maps.Keys(supported), compareIDs) {
		for _, kind := range _required {
			if !kinds[c][kind] {
				verr.Add("/entries", entity.FieldRequired, fmt.Sprintf("component %s of a supported language needs a row of %s", c, kind))
			}
		}
	}
	if len(verr.Fields) > 0 {
		return nil, verr
	}
	slices.SortFunc(out, compareEntries)
	return out, nil
}

// languageOf tells whether the profile checks the language; a profile without languages checks only rows without one.
func languageOf(p *entity.CheckProfile, language string) bool {
	switch {
	case p.AnyLanguage:
		return language != ""
	case len(p.Languages) == 0:
		return language == ""
	}
	return slices.Contains(p.Languages, language)
}

// builtinLanguage tells whether a builtin profile of the catalog checks the language.
func (u *UseCase) builtinLanguage(language string) bool {
	for i := range u.d.Catalog.Profiles {
		if p := &u.d.Catalog.Profiles[i]; p.Mode == entity.CheckModeBuiltin && slices.Contains(p.Languages, language) {
			return true
		}
	}
	return false
}

// uncovered are the components without a row and the rows whose profile the catalog of the release no longer has.
func (u *UseCase) uncovered(entries []entity.ProfileEntry, components []entity.Component) []entity.Uncovered {
	out := []entity.Uncovered{}
	has := map[entity.ID]bool{}
	for i := range entries {
		e := &entries[i]
		has[e.ComponentID] = true
		if _, ok := u.profiles[e.ProfileID]; !ok {
			out = append(out, entity.Uncovered{ComponentID: e.ComponentID, Language: e.Language, ReasonCode: entity.ReasonProfileUnavailable})
		}
	}
	for i := range components {
		if !has[components[i].ID] {
			out = append(out, entity.Uncovered{ComponentID: components[i].ID, ReasonCode: entity.ReasonProfileMissing})
		}
	}
	return out
}

// contentHash is SHA-256 of the sorted rows by their choice of profile; the versions of tools come from the catalog.
func contentHash(entries []entity.ProfileEntry) []byte {
	h := sha256.New()
	for i := range entries {
		e := &entries[i]
		for _, part := range [][]byte{e.ComponentID[:], []byte(e.Language), []byte(e.AnalysisKind), []byte(e.Mode), []byte(e.ProfileID)} {
			_, _ = h.Write(part)
			_, _ = h.Write([]byte{0})
		}
	}
	return h.Sum(nil)
}

func compareEntries(a, b entity.ProfileEntry) int {
	return cmp.Or(compareIDs(a.ComponentID, b.ComponentID), cmp.Compare(a.Language, b.Language), cmp.Compare(a.AnalysisKind, b.AnalysisKind))
}

func compareIDs(a, b entity.ID) int { return bytes.Compare(a[:], b[:]) }
