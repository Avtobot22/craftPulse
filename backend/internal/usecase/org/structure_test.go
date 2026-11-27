package org

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestPlanComponents_PreservesUndecidedProposals(t *testing.T) {
	t.Parallel()

	serviceID, repositoryID, proposalID, confirmedID := entity.ID{1}, entity.ID{2}, entity.ID{3}, entity.ID{4}
	current := []entity.Component{
		{ID: proposalID, ServiceID: serviceID, RepositoryAreaID: repositoryID, Status: entity.ComponentProposed, Origin: entity.ComponentManifest},
		{ID: confirmedID, ServiceID: serviceID, RepositoryAreaID: repositoryID, PathPrefix: "apps/api", Status: entity.ComponentConfirmed},
	}
	services := map[entity.ID]bool{serviceID: true}
	repositories := map[entity.ID]bool{repositoryID: true}
	unchanged := entity.ComponentInput{
		ID: confirmedID, ServiceID: serviceID, RepositoryAreaID: repositoryID, PathPrefix: "apps/api", Status: entity.ComponentConfirmed,
	}
	uc := UseCase{}
	var plan structurePlan
	var validation entity.ValidationError

	require.NoError(t, uc.planComponents(&plan, &validation, current,
		entity.StructureInput{Components: []entity.ComponentInput{unchanged}}, services, repositories, entity.Confirmation{}))
	assert.Empty(t, validation.Fields)
	assert.Empty(t, plan.components)

	require.NoError(t, uc.planComponents(&plan, &validation, current,
		entity.StructureInput{}, services, repositories, entity.Confirmation{}))
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "/components", validation.Fields[0].Field)
	assert.Equal(t, entity.FieldRequired, validation.Fields[0].Code)
}
