package app

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/view"
)

func taskViews(facts view.TaskFacts, structure view.TaskStructure, settings view.TaskSettings, collection view.TaskCollection, calculator *calc.UseCase) view.TaskDeps {
	return view.TaskDeps{
		Facts: facts, Structure: structure, Settings: settings, Collection: collection, Calculator: calculator, Revisions: calculator,
		Observations: []view.TaskObservationProvider{func(_ context.Context, _ entity.Subject, _ entity.Scope, f *entity.TaskObservationFacts) ([]entity.TaskObservation, bool, error) {
			return calc.TaskObservations(f), false, nil
		}},
	}
}
