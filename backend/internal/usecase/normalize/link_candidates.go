package normalize

import (
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

var taskKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]*-\d+`)

func taskKeys(text string) []string {
	var out []string
	for _, at := range taskKeyPattern.FindAllStringIndex(text, -1) {
		before, _ := utf8.DecodeLastRuneInString(text[:at[0]])
		after, _ := utf8.DecodeRuneInString(text[at[1]:])
		word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
		if word(before) || word(after) {
			continue
		}
		key := text[at[0]:at[1]]
		if !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	return out
}

func (w *writer) linkCandidates(ctx context.Context) error {
	if len(w.linkMRIDs) == 0 && len(w.linkTaskKeys) == 0 {
		return nil
	}
	links, err := w.u.d.Accounts.ProjectAreas(ctx, w.org, w.now)
	if err != nil {
		return err
	}
	byProject := map[entity.ID][]entity.ID{}
	affected := map[entity.ID]bool{}
	for _, l := range links {
		byProject[l.From] = append(byProject[l.From], l.To)
	}
	for a := range w.changes {
		affected[a] = true
	}
	areas := []entity.ID{}
	for a := range affected {
		areas = append(areas, a)
	}
	for _, group := range byProject {
		if slices.ContainsFunc(group, func(a entity.ID) bool { return affected[a] }) {
			areas = union(areas, group)
		}
	}
	mrs, err := w.u.d.Repo.LinkMRVersions(ctx, w.org, entity.LinkMRFilter{Areas: areas, MRIDs: w.linkMRIDs, TaskKeys: w.linkTaskKeys})
	if err != nil {
		return err
	}
	areasByMR := map[entity.ID][]entity.ID{}
	allAreas := []entity.ID{}
	allKeys := map[string]bool{}
	for i := range mrs {
		mr := &mrs[i]
		for _, group := range byProject {
			if slices.Contains(group, mr.AreaID) {
				areasByMR[mr.MRID] = union(areasByMR[mr.MRID], group)
			}
		}
		allAreas = union(allAreas, areasByMR[mr.MRID])
		for _, key := range unionKeys(taskKeys(mr.SourceBranch), taskKeys(mr.Title)) {
			allKeys[key] = true
		}
	}
	tasks, err := w.u.d.Repo.LinkTaskVersions(ctx, w.org, allAreas, slices.Sorted(maps.Keys(allKeys)))
	if err != nil {
		return err
	}
	tasksByKey := map[string][]*entity.TaskVersion{}
	for i := range tasks {
		t := &tasks[i]
		tasksByKey[t.TaskKey] = append(tasksByKey[t.TaskKey], t)
	}
	for i := range mrs {
		mr := &mrs[i]
		fields := []struct {
			value, field string
			method       entity.LinkMethod
		}{
			{mr.SourceBranch, "source_branch", entity.LinkBranchKey}, {mr.Title, "title", entity.LinkTitleKey},
		}
		byKey := map[string][]entity.ID{}
		for _, key := range unionKeys(taskKeys(mr.SourceBranch), taskKeys(mr.Title)) {
			for _, t := range tasksByKey[key] {
				if slices.Contains(areasByMR[mr.MRID], t.AreaID) {
					byKey[key] = append(byKey[key], t.TaskID)
				}
			}
		}
		var proposals []entity.LinkProposal
		for _, f := range fields {
			for _, key := range taskKeys(f.value) {
				for _, task := range byKey[key] {
					evidence, err := json.Marshal(struct {
						Field     string `json:"field"`
						Fragment  string `json:"fragment"`
						Ambiguous bool   `json:"ambiguous"`
					}{f.field, key, len(byKey[key]) > 1})
					if err != nil {
						return err
					}
					proposals = append(proposals, entity.LinkProposal{Type: entity.LinkTaskMR, From: task, To: mr.MRID, Method: f.method, Record: &mr.Record, Evidence: evidence})
				}
			}
		}
		if err := w.u.d.Links.ReconcileCandidates(ctx, w.org, mr.MRID, proposals); err != nil {
			return err
		}
	}
	return nil
}

func unionKeys(a, b []string) []string {
	for _, key := range b {
		if !slices.Contains(a, key) {
			a = append(a, key)
		}
	}
	return a
}
