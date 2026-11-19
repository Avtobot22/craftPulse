package normalize

import (
	"context"
	"encoding/json"
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
	mrs, err := w.u.d.Repo.LinkMRVersions(ctx, w.org, areas)
	if err != nil {
		return err
	}
	for i := range mrs {
		mr := &mrs[i]
		taskAreas := []entity.ID{}
		for _, group := range byProject {
			if slices.Contains(group, mr.AreaID) {
				taskAreas = union(taskAreas, group)
			}
		}
		fields := []struct {
			value, field string
			method       entity.LinkMethod
		}{
			{mr.SourceBranch, "source_branch", entity.LinkBranchKey}, {mr.Title, "title", entity.LinkTitleKey},
		}
		keys := unionKeys(taskKeys(mr.SourceBranch), taskKeys(mr.Title))
		tasks, err := w.u.d.Repo.LinkTaskVersions(ctx, w.org, taskAreas, keys)
		if err != nil {
			return err
		}
		byKey := map[string][]entity.ID{}
		for i := range tasks {
			t := &tasks[i]
			byKey[t.TaskKey] = append(byKey[t.TaskKey], t.TaskID)
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
