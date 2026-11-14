package process

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// WorkflowCatalog lists the statuses, resolutions, types and fields of every instance of the projects of a team,
// each value once, and the statuses without a rule in the current mapping (FR-017). The category of the source
// is shown as a hint and gives no meaning.
func (u *UseCase) WorkflowCatalog(ctx context.Context, s entity.Subject, teamID entity.ID) (entity.WorkflowCatalog, error) {
	org := s.OrganizationID
	areas, err := u.d.Teams.TeamAreas(ctx, org, teamID, u.d.Clock.Now())
	if err != nil {
		return entity.WorkflowCatalog{}, err
	}
	dicts, err := u.d.Dictionaries.Dictionaries(ctx, org, areas)
	if err != nil {
		return entity.WorkflowCatalog{}, err
	}
	var current entity.MappingVersion
	err = u.d.Tx.InOrgTx(ctx, org, func(ctx context.Context) error {
		var err error
		current, err = u.d.Repo.MappingVersionAt(ctx, org, teamID, math.MaxInt64)
		if isNotFound(err) {
			return nil
		}
		return err
	})
	if err != nil {
		return entity.WorkflowCatalog{}, err
	}
	out := entity.WorkflowCatalog{Instances: catalogInstances(dicts), Unmapped: []entity.NativeStatusRef{}}
	mapped := make(map[entity.NativeStatusRef]bool, len(current.Rules))
	for _, r := range current.Rules {
		if r.Kind == entity.RuleStatus {
			mapped[entity.NativeStatusRef{InstanceID: r.InstanceID, NativeID: r.NativeRef}] = true
		}
	}
	for _, inst := range out.Instances {
		for _, st := range inst.Statuses {
			ref := entity.NativeStatusRef{InstanceID: inst.InstanceID, NativeID: st.NativeID}
			if !mapped[ref] {
				out.Unmapped = append(out.Unmapped, ref)
			}
		}
	}
	return out, nil
}

// catalogInstances merges the areas of one instance: a status shared by projects of the instance is one value.
func catalogInstances(dicts []entity.AreaDictionary) []entity.CatalogInstance {
	type item struct {
		kind     entity.DictionaryKind
		nativeID string
	}
	byInstance := map[entity.ID]*entity.CatalogInstance{}
	seen := map[entity.ID]map[item]bool{}
	for _, d := range dicts {
		inst, ok := byInstance[d.InstanceID]
		if !ok {
			inst = &entity.CatalogInstance{
				InstanceID: d.InstanceID, Statuses: []entity.DictionaryItem{}, Resolutions: []entity.DictionaryItem{},
				Types: []entity.DictionaryItem{}, Fields: []entity.DictionaryItem{},
			}
			byInstance[d.InstanceID], seen[d.InstanceID] = inst, map[item]bool{}
		}
		for _, it := range d.Items {
			k := item{it.Kind, it.NativeID}
			if seen[d.InstanceID][k] {
				continue
			}
			seen[d.InstanceID][k] = true
			switch it.Kind {
			case entity.DictionaryStatus:
				inst.Statuses = append(inst.Statuses, it)
			case entity.DictionaryResolution:
				inst.Resolutions = append(inst.Resolutions, it)
			case entity.DictionaryType:
				inst.Types = append(inst.Types, it)
			case entity.DictionaryField:
				inst.Fields = append(inst.Fields, it)
			}
		}
	}
	out := make([]entity.CatalogInstance, 0, len(byInstance))
	for _, inst := range byInstance {
		for _, items := range [][]entity.DictionaryItem{inst.Statuses, inst.Resolutions, inst.Types, inst.Fields} {
			slices.SortFunc(items, func(a, b entity.DictionaryItem) int { return cmp.Compare(a.NativeID, b.NativeID) })
		}
		out = append(out, *inst)
	}
	slices.SortFunc(out, func(a, b entity.CatalogInstance) int { return bytes.Compare(a.InstanceID[:], b.InstanceID[:]) })
	return out
}
