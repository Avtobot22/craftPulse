package normalize

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
)

func TestTaskKeys(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		text string
		want []string
	}{
		{"feature/PA-7-cache", []string{"PA-7"}},
		{"Fix PA-7 and XY_2-99", []string{"PA-7", "XY_2-99"}},
		{"xPA-7 PA-7suffix PA-70x", nil},
		{"PA-7 PA-7", []string{"PA-7"}},
		{"Без ключа", nil},
	} {
		assert.Equal(t, tt.want, taskKeys(tt.text), tt.text)
	}
}

type candidateAccounts struct {
	Accounts
	links []entity.Link
}

func (a candidateAccounts) ProjectAreas(context.Context, entity.ID, time.Time) ([]entity.Link, error) {
	return a.links, nil
}

type candidateRepo struct {
	repo.NormalizeRepo
	filter    entity.LinkMRFilter
	taskReads int
	mrs       []entity.MRVersion
}

func (r *candidateRepo) LinkMRVersions(_ context.Context, _ entity.ID, f entity.LinkMRFilter) ([]entity.MRVersion, error) {
	r.filter = f
	return r.mrs, nil
}

func (r *candidateRepo) LinkTaskVersions(context.Context, entity.ID, []entity.ID, []string) ([]entity.TaskVersion, error) {
	r.taskReads++
	return nil, nil
}

type candidateLinks struct {
	Links
	targets []entity.ID
}

func (l *candidateLinks) ReconcileCandidates(_ context.Context, _ entity.ID, mr entity.ID, _ []entity.LinkProposal) error {
	l.targets = append(l.targets, mr)
	return nil
}

func TestLinkCandidates_TargetsChangesAndReadsTasksOnce(t *testing.T) {
	t.Parallel()
	project, repository, tracker := entity.ID{1}, entity.ID{2}, entity.ID{3}
	first, second := entity.ID{4}, entity.ID{5}
	r := &candidateRepo{mrs: []entity.MRVersion{{MRID: first, AreaID: repository, Title: "PA-7"}, {MRID: second, AreaID: repository, Title: "PA-7"}}}
	l := &candidateLinks{}
	a := candidateAccounts{links: []entity.Link{{From: project, To: repository}, {From: project, To: tracker}}}
	w := writer{u: New(Deps{Repo: r, Accounts: a, Links: l}), changes: changes{tracker: &entity.AreaChange{AreaID: tracker}}, linkMRIDs: []entity.ID{first}, linkTaskKeys: []string{"PA-7"}}
	assert.NoError(t, w.linkCandidates(t.Context()))
	assert.Equal(t, []entity.ID{first}, r.filter.MRIDs)
	assert.Equal(t, []string{"PA-7"}, r.filter.TaskKeys)
	assert.Contains(t, r.filter.Areas, repository)
	assert.Equal(t, 1, r.taskReads)
	assert.Equal(t, []entity.ID{first, second}, l.targets)
}
