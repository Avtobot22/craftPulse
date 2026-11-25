package access

import (
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// A task key only grants the list operation and only the projects both the owner and the key may read.
func taskKeyScope(s entity.Subject, r entity.AccessRequest, p entity.Profile) (entity.Scope, error) {
	if r.Operation != "listTasks" || r.Action != "tasks.read" || !granted(p, r.Action) {
		return entity.Scope{}, entity.ErrForbidden
	}
	projects := make([]entity.ID, 0, len(s.APIKeyProjects))
	for _, project := range p.Areas.Projects {
		if slices.Contains(s.APIKeyProjects, project.ID) {
			projects = append(projects, project.ID)
		}
	}
	return entity.GrantScope(entity.ScopeGrant{Projects: projects}), nil
}
