package calc

import (
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// TaskBelongsToTeam applies the same team field rule as the team report.
// The caller first selects teams that own the project.
func (*UseCase) TaskBelongsToTeam(f *entity.TaskObservationFacts, team entity.ID) bool {
	if f.Mapping == nil {
		return true
	}
	rs := rulesOf(f.Mapping)[f.InstanceID]
	return rs == nil || rs.ofTeam(&f.Task, team)
}

// TaskWorkflow projects T14 and the T15 intervals from the same timeline as task observations.
func (*UseCase) TaskWorkflow(f *entity.TaskObservationFacts) entity.TaskWorkflow {
	out := entity.TaskWorkflow{Meaning: "unknown"}
	tl := observationTimeline(f)
	if tl == nil {
		return out
	}
	if meaning := tl.rules.status[f.Task.StatusNativeID]; meaning != "" && meaning != entity.MeaningIgnored {
		out.Meaning = meaning
	}
	out.Type, out.Size = tl.slice.TaskType, tl.slice.SizeLabel
	ep, group := taskEpisode(tl, f.AsOf)
	out.Target = observationTarget(f, group)
	if ep != nil && ep.startKnown {
		duration := f.AsOf.Sub(ep.start)
		age := int(duration / time.Second)
		out.AgeSeconds = &age
		if out.Target != nil {
			over := duration.Seconds() > float64(out.Target.DurationSeconds)
			out.OverTarget = &over
		}
	}
	if tl.knownAt(f.AsOf) {
		out.Blocked = taskBlocks(tl)
	}
	return out
}

func taskBlocks(tl *timeline) []entity.TaskBlock {
	var out []entity.TaskBlock
	for _, e := range tl.episodes {
		for i, s := range e.segments {
			if s.class != entity.IntervalBlocked {
				continue
			}
			end := utc(e.end)
			if i+1 < len(e.segments) {
				end = utc(&e.segments[i+1].from)
			}
			out = append(out, entity.TaskBlock{From: s.from.UTC(), To: end})
		}
	}
	return out
}

// SortTaskObservations applies the card's ordering across all providers.
func (*UseCase) SortTaskObservations(out []entity.TaskObservation) {
	sortTaskObservations(out)
}

func taskEpisode(tl *timeline, at time.Time) (*episode, *entity.TargetGroup) {
	if tl == nil || !tl.knownAt(at) {
		return nil, nil
	}
	if tl.slice.TaskType == nil || tl.slice.SizeLabel == nil {
		return tl.episodeAt(at), nil
	}
	return tl.episodeAt(at), &entity.TargetGroup{TaskType: *tl.slice.TaskType, SizeLabel: *tl.slice.SizeLabel}
}
