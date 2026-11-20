package calc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _research = "research"

// codeMapping is m1 with the stories as the code-required type.
func codeMapping() *entity.MappingVersion {
	m := m1()
	m.Rules = append(m.Rules, entity.MappingRule{
		Kind: entity.RuleCodeRequiredType, InstanceID: _instance, NativeRef: _story, Meaning: entity.MeaningCodeRequired,
	})
	return m
}

func typed(s taskSpec, typ string) taskInput {
	in := s.input()
	in.version.TypeNativeID = typ
	return in
}

// linkTasks are the tasks of October: A is a story completed on d5, B a story in work at the end, C research in
// work, D a story that has not started.
func linkTasks() []taskInput {
	return []taskInput{
		typed(task("A", d(0), _intake, status(d(1), _active), status(d(5), _done)), _story),
		typed(task("B", d(0), _intake, status(d(2), _active)), _story),
		typed(task("C", d(0), _intake, status(d(1), _active)), _research),
		typed(task("D", d(0), _intake), _story),
	}
}

func mrVersion(name string, opened time.Time, merged *time.Time) entity.MRVersion {
	state := "open"
	if merged != nil {
		state = "merged"
	}
	return entity.MRVersion{ID: idOf(name + "/v"), MRID: idOf(name), OpenedAt: opened, MergedAt: merged, State: state}
}

// linkMRs are the MRs of the repository of the team: M1 merged on d4, M2 open, M3 merged on d6 without a task.
func linkMRs() []entity.ReviewFacts {
	merged4, merged6 := d(4), d(6)
	return []entity.ReviewFacts{{MRs: []entity.MRVersion{
		mrVersion("M1", d(1), &merged4), mrVersion("M2", d(2), nil), mrVersion("M3", d(3), &merged6),
	}}}
}

func edge(task, mr string, method entity.LinkMethod, status entity.LinkStatus) entity.LinkEdge {
	return entity.LinkEdge{ID: idOf(task + "-" + mr + "/" + string(status)), Type: entity.LinkTaskMR, From: idOf(task), To: idOf(mr), Method: method, Status: status}
}

// linkPoints computes the only point of October of A18, A20 and T28.
func linkPoints(t *testing.T, mapping *entity.MappingVersion, edges ...entity.LinkEdge) map[string]entity.Point {
	t.Helper()
	w := entity.Window{From: d(0), To: d(31), Step: entity.StepMonth}
	in := input{mapping: mapping, team: _team, window: w, asOf: d(31), location: time.UTC, tasks: linkTasks()}
	li := linkInputOf(in, linkMRs(), edges)
	out := map[string]entity.Point{}
	for _, s := range li.series(periods(w, in.asOf, time.UTC)) {
		require.Len(t, s.Points, 1, s.MetricID)
		out[s.MetricID] = s.Points[0]
	}
	return out
}

func numerators(p entity.Point) []entity.ID {
	var out []entity.ID
	for _, c := range p.Contributions {
		if c.Role == entity.RoleNumerator {
			out = append(out, c.ObjectID)
		}
	}
	return out
}

// TestLinks_V17_OnlyConfirmedLinksCount is V-17: a key in a branch gives a candidate that counts nowhere; the
// confirmation counts the MR in A18 and the task in A20 and T28, a rejection takes them out again.
func TestLinks_V17_OnlyConfirmedLinksCount(t *testing.T) {
	tests := []struct {
		name   string
		edges  []entity.LinkEdge
		linked bool
		reason string
	}{
		{name: "candidate by key", edges: []entity.LinkEdge{edge("A", "M1", entity.LinkBranchKey, entity.LinkCandidate)}, reason: _linkCandidate},
		{name: "confirmed", edges: []entity.LinkEdge{edge("A", "M1", entity.LinkUserConfirmed, entity.LinkConfirmed)}, linked: true, reason: string(entity.LinkUserConfirmed)},
		{name: "rejected", edges: []entity.LinkEdge{edge("A", "M1", entity.LinkBranchKey, entity.LinkRejected)}, reason: _linkRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pts := linkPoints(t, codeMapping(), tt.edges...)

			a18, a20, t28 := pts["A18"], pts["A20"], pts["T28"]
			require.Equal(t, entity.StateAssessed, a18.State)
			assert.Equal(t, 2.0, *a18.Denominator, "M1 and M3 are merged in October, M2 is open")
			assert.Equal(t, 2.0, *a20.Denominator, "A completed and B in work are stories; C is research, D has not started")
			assert.Equal(t, 2.0, *t28.Denominator)
			assert.Equal(t, tt.reason, contributions(a18, idOf("M1"))[0].ReasonCode)
			assert.Equal(t, tt.reason, contributions(t28, idOf("A"))[0].ReasonCode)
			if !tt.linked {
				assert.Empty(t, numerators(a18))
				assert.Empty(t, numerators(a20))
				assert.Empty(t, numerators(t28))
				assert.Equal(t, 0.0, *a18.Value)
				return
			}
			assert.Equal(t, []entity.ID{idOf("M1")}, numerators(a18))
			assert.Equal(t, []entity.ID{idOf("A")}, numerators(a20))
			assert.Equal(t, []entity.ID{idOf("A")}, numerators(t28))
			assert.Equal(t, 0.5, *a18.Value)
			assert.Equal(t, 0.5, *t28.Value)
		})
	}
}

// TestLinks_OpenMRCountsForChainOnly: a confirmed open MR is a chain for A20 and no traceability for T28 until
// it is merged.
func TestLinks_OpenMRCountsForChainOnly(t *testing.T) {
	pts := linkPoints(t, codeMapping(), edge("B", "M2", entity.LinkUserConfirmed, entity.LinkConfirmed))

	assert.Equal(t, []entity.ID{idOf("B")}, numerators(pts["A20"]))
	assert.Equal(t, 0.5, *pts["A20"].Value)
	assert.Empty(t, numerators(pts["T28"]))
	assert.Equal(t, 0.0, *pts["T28"].Value)
	assert.Empty(t, numerators(pts["A18"]), "M2 is not merged")
}

// TestLinks_TypesOutsideThePolicy: research needs no MR, so it is excluded from A20 and NOT_APPLICABLE in T28;
// without code_required_type rules or without a mapping A20 and T28 are NOT_EVALUATED, A18 still counts.
func TestLinks_TypesOutsideThePolicy(t *testing.T) {
	pts := linkPoints(t, codeMapping(), edge("C", "M1", entity.LinkUserConfirmed, entity.LinkConfirmed))
	for _, metric := range []string{"A20", "T28"} {
		c := contributions(pts[metric], idOf("C"))
		require.Len(t, c, 1, metric)
		assert.Equal(t, entity.RoleExcluded, c[0].Role)
		assert.Equal(t, entity.ReasonNotCodeRequired, c[0].ReasonCode)
		assert.Equal(t, 1, pts[metric].ExcludedCount)
	}
	assert.Equal(t, []entity.ID{idOf("M1")}, numerators(pts["A18"]), "the task of the link may be of any type")

	for _, tt := range []struct {
		name    string
		mapping *entity.MappingVersion
		reason  string
	}{
		{"no code_required_type rules", m1(), entity.ReasonCodePolicyMissing},
		{"no mapping", nil, entity.ReasonMappingMissing},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pts := linkPoints(t, tt.mapping, edge("A", "M1", entity.LinkUserConfirmed, entity.LinkConfirmed))
			for _, metric := range []string{"A20", "T28"} {
				assert.Equal(t, entity.StateNotEvaluated, pts[metric].State, metric)
				assert.Equal(t, tt.reason, pts[metric].ReasonCode, metric)
				assert.Nil(t, pts[metric].Value, metric)
			}
			assert.Equal(t, entity.StateAssessed, pts["A18"].State)
			assert.Equal(t, 0.5, *pts["A18"].Value)
		})
	}
}

// TestLinks_ConfirmedMROutsideTheTeamIsUnknown: the merge of an MR outside the repositories of the team is not
// known, so T28 is provisional with its task missing, while A20 counts the confirmed chain.
func TestLinks_ConfirmedMROutsideTheTeamIsUnknown(t *testing.T) {
	pts := linkPoints(t, codeMapping(), edge("A", "Other", entity.LinkSourceExplicit, entity.LinkConfirmed))

	t28 := pts["T28"]
	assert.Equal(t, entity.StateProvisional, t28.State)
	assert.Equal(t, entity.ReasonMRUnknown, t28.ReasonCode)
	assert.Equal(t, 1, t28.MissingCount)
	assert.Equal(t, entity.StateProvisional, pts["A20"].State)
}

func TestLinks_HashFollowsLinkVersions(t *testing.T) {
	w := entity.Window{From: d(0), To: d(31), Step: entity.StepMonth}
	series := &entity.CalcSeries{ScopeHash: []byte("team"), Step: entity.StepMonth}
	hash := func(edges ...entity.LinkEdge) []byte {
		in := input{mapping: codeMapping(), team: _team, window: w, asOf: d(31), location: time.UTC, tasks: linkTasks()}
		src := &linkSource{in: linkInputOf(in, linkMRs(), edges), series: series}
		return src.hash(d(0), d(31))
	}
	candidate := hash(edge("A", "M1", entity.LinkBranchKey, entity.LinkCandidate))

	assert.Equal(t, candidate, hash(edge("A", "M1", entity.LinkBranchKey, entity.LinkCandidate)))
	assert.NotEqual(t, candidate, hash(edge("A", "M1", entity.LinkUserConfirmed, entity.LinkConfirmed)))
}
