package logger_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

func TestNewLogger_MasksRawAndEscapedSecrets(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	secret := `pa"ss\word`
	log, err := logger.New(&out, "info", "api", []string{secret, ""})
	require.NoError(t, err)

	log.Info().Str("value", secret).Msg("connect with " + secret)

	assert.NotContains(t, out.String(), `pa\"ss\\word`)
	assert.NotContains(t, out.String(), secret)
	assert.Contains(t, out.String(), "[REDACTED]")
	assert.Contains(t, out.String(), `"service":"api"`)
}

func TestWithCorrelation(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	ctx := logger.WithCorrelation(context.Background(), zerolog.New(&out))

	id := logger.CorrelationID(ctx)
	zerolog.Ctx(ctx).Info().Msg("job")

	assert.NotEqual(t, uuid.Nil, id)
	assert.Contains(t, out.String(), id.String())
	assert.Equal(t, uuid.Nil, logger.CorrelationID(context.Background()))
}
