package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type probeRunner struct {
	err    error
	failed int
}

func (r *probeRunner) RunProbe(context.Context, entity.ID, entity.ID, entity.ID) error { return r.err }

func (r *probeRunner) FailProbe(context.Context, entity.ID, entity.ID, entity.ID) error {
	r.failed++
	return nil
}

func TestProbeWorker_FailsProbeAfterLastAttempt(t *testing.T) {
	t.Parallel()
	boom := errors.New("open adapter key")
	tests := []struct {
		name       string
		err        error
		attempt    int
		wantFailed int
	}{
		{name: "success", attempt: 25},
		{name: "attempts left", err: boom, attempt: 3},
		{name: "last attempt", err: boom, attempt: 25, wantFailed: 1},
		{name: "rate limit waits", err: &entity.RetryLaterError{At: time.Now().Add(time.Minute), Err: boom}, attempt: 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &probeRunner{err: tt.err}
			w := &probeWorker{runner: r}
			err := w.Work(t.Context(), &river.Job[entity.ConnectProbeJob]{JobRow: &rivertype.JobRow{Attempt: tt.attempt, MaxAttempts: 25}})
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, tt.wantFailed, r.failed)
		})
	}
}
