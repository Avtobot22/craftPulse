package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

func TestManual(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	c := clock.NewManual(start)

	c.Advance(time.Minute)

	assert.Equal(t, start.Add(time.Minute), c.Now())
}
