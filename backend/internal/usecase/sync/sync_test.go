package sync_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
)

func TestPause(t *testing.T) {
	t.Parallel()
	for retry, base := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second} {
		assert.Equal(t, base, sync.Pause(retry, 0), "retry %d without jitter", retry)
		assert.Equal(t, base+base/2, sync.Pause(retry, 1), "retry %d with the most jitter", retry)
		assert.Equal(t, base+base/4, sync.Pause(retry, 0.5))
	}
}
