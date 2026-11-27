package connect

import (
	"context"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// sonarCapability combines the adapter's edition check with the local repository bindings.
func (u *UseCase) sonarCapability(ctx context.Context, org entity.ID, state *entity.ConnectionState) error {
	i := slices.IndexFunc(state.Capabilities, func(c entity.Capability) bool { return c.Name == entity.CapabilityReadMRAnalysis })
	if i < 0 {
		i = len(state.Capabilities)
		state.Capabilities = append(state.Capabilities, entity.Capability{
			Name: entity.CapabilityReadMRAnalysis, State: entity.CapabilityMissing, ReasonCode: entity.CapabilityNotChecked,
			CheckedAt: state.Connection.CreatedAt,
		})
	}
	c := &state.Capabilities[i]
	// Preserve explicit adapter reasons, including an edition without MR analysis.
	if c.State != entity.CapabilityVerified && c.ReasonCode != "" && c.ReasonCode != entity.CapabilityNotChecked {
		return nil
	}
	var bound, unbound bool
	for i := range state.Scopes {
		sc := &state.Scopes[i]
		if !sc.Active || sc.Kind != entity.ScopeAnalysisProject {
			continue
		}
		repository, err := u.d.Repo.SonarRepository(ctx, org, sc.ID)
		if err != nil {
			return err
		}
		bound = bound || repository != nil
		unbound = unbound || repository == nil
	}
	if unbound {
		if bound && c.State == entity.CapabilityVerified {
			c.State = entity.CapabilityPartial
		} else {
			c.State = entity.CapabilityMissing
		}
		c.ReasonCode = "repository_binding_missing"
	}
	return nil
}
