package view

import (
	"bytes"
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

// DocumentationRoots reads the confirmed roots and their normalized inventory through docs.
type DocumentationRoots interface {
	WikiInventoryAt(context.Context, entity.Subject, entity.Scope, entity.DocumentationScope, int64) (*entity.DocumentationInventory, []entity.AccessNote, error)
	ListRoots(context.Context, entity.Subject, entity.Scope, entity.ID) (entity.DocumentationRootSet, error)
	InventoryAt(context.Context, entity.ID, entity.Scope, entity.DocumentationScope, int64) (*entity.DocumentationInventory, error)
}

// DocumentationReports selects check evidence and reads repository metadata.
type DocumentationReports interface {
	DocumentationCheckCompleted(context.Context, entity.ID, entity.DocumentationRoot, int64, int64, time.Time) (bool, error)
	LatestDocumentationChecks(context.Context, entity.ID, entity.DocumentationRoot, int64, int64) ([]entity.DocumentationCheck, error)
}

// DocumentationFindings projects the open findings of a root's report stream.
type DocumentationFindings interface {
	OpenDocumentationViolations(context.Context, entity.ID, []entity.ID) (map[string]int, error)
}

// DocumentationDeps wires the readers used by the service documentation landing page.
type DocumentationDeps struct {
	Roots    DocumentationRoots
	Reports  DocumentationReports
	Findings DocumentationFindings
	Access   usecase.Access
	Tenants  repo.TenantRepo
}

// ListServiceDocuments reads current documents from confirmed and visible roots.
func (u *UseCase) ListServiceDocuments(ctx context.Context, s entity.Subject, scope entity.Scope, service, after entity.ID, limit int) (entity.ServiceDocuments, error) {
	out := entity.ServiceDocuments{State: entity.StateUnknown}
	d := u.d.Documentation
	set, err := d.Roots.ListRoots(ctx, s, scope, service)
	if err != nil || len(set.Roots) == 0 {
		return out, err
	}
	areas := make([]entity.ID, 0, len(set.Roots))
	for i := range set.Roots {
		areas = append(areas, set.Roots[i].AreaID)
	}
	scope, err = d.Access.Decide(ctx, s, scope, entity.SourceQuery{Operation: "listServiceDocuments", Areas: areas})
	if err != nil {
		return out, err
	}
	out.Notes = scope.Notes()
	var observed, completed int
	items := map[entity.ID]*entity.ServiceDocument{}
	org := s.OrganizationID
	var seq int64
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		seq, _, err = d.Tenants.Clock(ctx, org)
		if err != nil {
			return err
		}
		for i := range set.Roots {
			root := set.Roots[i]
			if root.ProfileID == entity.WikiInventoryProfile || !scope.Area(root.AreaID) {
				continue
			}
			checks, err := d.Reports.LatestDocumentationChecks(ctx, org, root, seq, 0)
			if err != nil {
				return err
			}
			if len(checks) == 0 {
				continue
			}
			observed++
			if documentationComplete(checks) {
				completed++
			}
			if err := u.addRootDocuments(ctx, org, scope, root, seq, checks, items); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return entity.ServiceDocuments{}, err
	}
	// Permission checks may query the source and need their own connections.
	for i := range set.Roots {
		root := &set.Roots[i]
		if root.ProfileID != entity.WikiInventoryProfile || !scope.Area(root.AreaID) {
			continue
		}
		cut := entity.DocumentationScope{ServiceID: service, RootID: root.ID, RepositoryID: root.AreaID}
		inventory, notes, err := d.Roots.WikiInventoryAt(ctx, s, scope, cut, seq)
		if err != nil {
			return entity.ServiceDocuments{}, err
		}
		for _, note := range notes {
			if !slices.Contains(out.Notes, note) {
				out.Notes = append(out.Notes, note)
			}
		}
		if inventory == nil || len(inventory.Documents) == 0 {
			continue
		}
		observed++
		for j := range inventory.Documents {
			doc := &inventory.Documents[j]
			items[doc.DocumentID] = &entity.ServiceDocument{DocumentVersion: *doc, SourceURL: doc.SourceURL}
		}
	}
	switch {
	case completed == len(set.Roots):
		out.State = entity.StateAssessed
	case observed > 0:
		out.State = entity.StateProvisional
	case len(out.Notes) == 0:
		out.State = entity.StateNotEvaluated
	}
	for id, doc := range items {
		if bytes.Compare(id[:], after[:]) > 0 {
			out.Items = append(out.Items, *doc)
		}
	}
	slices.SortFunc(out.Items, func(a, b entity.ServiceDocument) int { return bytes.Compare(a.DocumentID[:], b.DocumentID[:]) })
	if len(out.Items) > limit {
		out.Next = &out.Items[limit-1].DocumentID
		out.Items = out.Items[:limit]
	}
	return out, nil
}

func (u *UseCase) addRootDocuments(ctx context.Context, org entity.ID, scope entity.Scope, root entity.DocumentationRoot,
	seq int64, checks []entity.DocumentationCheck, items map[entity.ID]*entity.ServiceDocument,
) error {
	d := u.d.Documentation
	cut := entity.DocumentationScope{ServiceID: root.ServiceID, RootID: root.ID, RepositoryID: root.AreaID, SHA: checks[0].SHA}
	inventory, err := d.Roots.InventoryAt(ctx, org, scope, cut, seq)
	if err != nil || inventory == nil || len(inventory.Documents) == 0 {
		return err
	}
	reportIDs, lastChecked := documentationEvidence(checks)
	counts, err := d.Findings.OpenDocumentationViolations(ctx, org, reportIDs)
	if err != nil {
		return err
	}
	for i := range inventory.Documents {
		doc := &inventory.Documents[i]
		if doc.ACLState != "known" {
			continue
		}
		item := &entity.ServiceDocument{DocumentVersion: *doc, LastCheckedAt: lastChecked, SourceURL: documentationURL(checks[0], doc.VersionRef, doc.Path)}
		if len(reportIDs) > 0 {
			item.OpenViolations = new(counts[doc.Path])
		}
		// Nested roots can contribute the same document. Use its most recent check.
		previous, exists := items[doc.DocumentID]
		if !exists || laterCheck(item.LastCheckedAt, previous.LastCheckedAt) {
			items[doc.DocumentID] = item
		}
	}
	return nil
}

func laterCheck(a, b *time.Time) bool {
	return a != nil && (b == nil || a.After(*b))
}

func documentationURL(check entity.DocumentationCheck, sha, path string) *string {
	if check.Provider != entity.ProviderGitLab {
		return nil
	}
	segments := strings.Split(path, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return gitLabCIURL(entity.SourceInstance{InstanceKey: check.InstanceKey}, entity.RepositoryVersion{WebURL: check.RepositoryURL},
		check.ProjectPath, "/-/blob/"+url.PathEscape(sha)+"/"+strings.Join(segments, "/"))
}

func documentationEvidence(checks []entity.DocumentationCheck) ([]entity.ID, *time.Time) {
	var ids []entity.ID
	var last *time.Time
	for i := range checks {
		check := &checks[i]
		if check.SARIFID.IsZero() {
			continue
		}
		ids = append(ids, check.SARIFID)
		if last == nil && check.Latest && check.SHA == checks[0].SHA {
			last = new(check.ObservedAt)
		}
	}
	return ids, last
}

func documentationComplete(checks []entity.DocumentationCheck) bool {
	if len(checks) == 0 {
		return false
	}
	kinds := checks[0].Checks
	if len(kinds) == 0 {
		kinds = []string{entity.AnalysisDocs, entity.AnalysisOpenAPI}
	}
	return !slices.ContainsFunc(kinds, func(kind string) bool {
		return !slices.ContainsFunc(checks, func(c entity.DocumentationCheck) bool {
			return c.Complete && c.Latest && c.SHA == checks[0].SHA && c.Kind == kind
		})
	})
}
