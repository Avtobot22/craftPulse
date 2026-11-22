package calc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCodeMetrics_ThresholdAndCompleteness(t *testing.T) {
	t.Parallel()
	src := codeSource{format: entity.CodeFormatLizard}
	p := period{start: time.Unix(0, 0), end: time.Unix(1, 0)}
	for _, pt := range src.points(p) {
		assert.Equal(t, entity.StateNotEvaluated, pt.State)
		assert.Nil(t, pt.Value)
	}
	src.report = &entity.CodeReport{
		Completeness: "partial", SkippedCount: 2, Skipped: []entity.SkippedTarget{{Path: "missing.go", Reason: "unsupported"}},
		Facts: entity.CodeFacts{
			Files:      new([]entity.CodeFile{{Path: "a.go", NLOC: 120}, {Path: "a_test.go", NLOC: 14, Test: true}}),
			Thresholds: &entity.CodeThresholds{CCN: 10, NLOC: 50},
			Functions:  []entity.CodeFunction{{Name: "at", CCN: 10, NLOC: 50}, {Name: "above", CCN: 11, NLOC: 51}},
		},
	}
	points := src.points(p)
	for _, tt := range []struct {
		id, est string
		value   float64
	}{
		{"C01", "production", 120},
		{"C01", "test", 14},
		{"C04", "p50", 10},
		{"C04", "p90", 11},
		{"C04", "max", 11},
		{"C05", "", .5},
		{"C06", "p50", 50},
		{"C06", "p90", 51},
		{"C06", "max", 51},
		{"C06", "above_threshold", 1},
	} {
		t.Run(tt.id+"/"+tt.est, func(t *testing.T) {
			got := codePoint(t, points, tt.id, tt.est)
			assert.Equal(t, entity.StateProvisional, got.State)
			require.NotNil(t, got.Value)
			assert.InDelta(t, tt.value, *got.Value, 0)
			assert.Equal(t, 2, got.MissingCount)
			assert.Contains(t, string(got.Contributions[len(got.Contributions)-1].Evidence), "missing.go")
		})
	}
	c := codePoint(t, points, "C05", "")
	assert.Equal(t, entity.RoleDenominator, c.Contributions[0].Role)
	assert.Equal(t, entity.RoleNumerator, c.Contributions[1].Role)
	assert.Equal(t, 1.0, *c.Numerator)
	assert.Equal(t, 2.0, *c.Denominator)

	src.report.Facts.Files, src.report.Facts.Thresholds = nil, nil
	points = src.points(p)
	assert.Equal(t, entity.StateNotEvaluated, codePoint(t, points, "C01", "production").State)
	assert.Equal(t, entity.StateNotEvaluated, codePoint(t, points, "C05", "").State)
	assert.Equal(t, entity.StateProvisional, codePoint(t, points, "C04", "max").State)
}

func TestCodeMetrics_UnionAcrossGroups(t *testing.T) {
	t.Parallel()
	src := codeSource{format: entity.CodeFormatJscpd, report: &entity.CodeReport{
		Completeness: "complete",
		Facts: entity.CodeFacts{
			PhysicalLines: new(int64(21)),
			Groups: []entity.CloneGroup{
				{Members: []entity.CodeRange{{Path: "a", Start: 1, End: 10}, {Path: "b", Start: 1, End: 10}}},
				{Members: []entity.CodeRange{{Path: "a", Start: 3, End: 10}, {Path: "b", Start: 4, End: 11}}},
			},
		},
	}}
	p := period{start: time.Unix(0, 0), end: time.Unix(1, 0)}
	points := src.points(p)
	coverage := codePoint(t, points, "C08", "")
	require.NotNil(t, coverage.Value)
	assert.Equal(t, 1.0, *coverage.Value)
	assert.Equal(t, 21.0, *coverage.Numerator)
	require.Len(t, coverage.Contributions, 3)
	assert.Equal(t, 20.0, *coverage.Contributions[0].Value)
	assert.Equal(t, 1.0, *coverage.Contributions[1].Value)
	assert.Equal(t, entity.RoleDenominator, coverage.Contributions[2].Role)

	groups := codePoint(t, points, "C09", "")
	assert.Equal(t, 2.0, *groups.Value)
	assert.Len(t, groups.Contributions, 2)
	*src.report.Facts.PhysicalLines = 20
	bad := codePoint(t, src.points(p), "C08", "")
	assert.Equal(t, entity.StateNotEvaluated, bad.State)
	assert.Nil(t, bad.Value)
}

func codePoint(t *testing.T, points []entity.SeriesPoint, id, est string) entity.SeriesPoint {
	t.Helper()
	for i := range points {
		point := &points[i]
		if point.MetricID == id && point.Estimator == est {
			return *point
		}
	}
	t.Fatalf("no point %s/%s", id, est)
	return entity.SeriesPoint{}
}
