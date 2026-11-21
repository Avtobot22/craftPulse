package org

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// LockStructureVersion serializes linking an import area with edits to the project structure.
// The caller has an organization transaction and locks the structure before taking the knowledge sequence.
func (u *UseCase) LockStructureVersion(ctx context.Context, orgID entity.ID) (int64, error) {
	return u.orgs.LockStructureVersion(ctx, orgID)
}

// LinkReportArea makes a normalized universal report visible through its project. The link, report
// and structure version commit together, with the same knowledge sequence.
func (u *UseCase) LinkReportArea(ctx context.Context, orgID, projectID, areaID entity.ID, version, seq int64) error {
	now := u.clock.Now()
	links, err := u.orgs.CurrentLinks(ctx, orgID, entity.LinkProjectArea, now)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(links, func(l entity.Link) bool { return l.From == projectID && l.To == areaID }) {
		return nil
	}
	link := entity.Link{From: projectID, To: areaID, ValidFrom: now}
	if err := u.orgs.InsertLink(ctx, orgID, entity.LinkProjectArea, link, seq); err != nil {
		return err
	}
	return u.orgs.SetStructureVersion(ctx, orgID, version+1)
}
