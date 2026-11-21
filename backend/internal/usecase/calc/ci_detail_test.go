package calc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestSummarizeRun_V28AndIncomplete(t *testing.T) {
	t.Parallel()
	facts, _ := ciFixture()
	run := entity.CIRun{Run: facts.Runs[0], CIAttempts: entity.CIAttempts{History: entity.AttemptHistoryComplete, Attempts: facts.Attempts}}
	u := New(Deps{})
	summary := u.SummarizeRun(run, facts.Provider)
	assert.Equal(t, 3, summary.AttemptsTotal)
	assert.Equal(t, 1, summary.RetriedJobs)
	assert.Zero(t, summary.AllowedFailures)
	require.NotNil(t, summary.UsageSeconds.Value)
	assert.Equal(t, 130.0, *summary.UsageSeconds.Value)
	run.Run.AttemptHistory = entity.AttemptHistoryTruncated
	bounded := u.SummarizeRun(run, facts.Provider)
	assert.Equal(t, entity.StateBounded, bounded.UsageSeconds.State)
	assert.Nil(t, bounded.UsageSeconds.Value)
	require.NotNil(t, bounded.UsageSeconds.Bound)
	assert.Equal(t, 130.0, bounded.UsageSeconds.Bound.Value)
	// A failed retry that is allowed to fail belongs to I05; a failed first attempt followed by pass does not.
	run.Attempts[1].NativeStatus, run.Attempts[1].AllowFailure = "failed", true
	assert.Equal(t, 1, u.SummarizeRun(run, facts.Provider).AllowedFailures)
	assert.Zero(t, u.SummarizeRun(run, entity.ProviderJenkins).AllowedFailures)
}
