package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/config"
)

const _marker = "MARKER-secret"

func secretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "password")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func validEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"DATABASE_URL":           "postgres://app@db:5432/craftpulse",
		"DATABASE_PASSWORD_FILE": secretFile(t, _marker+"\n"),
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(validEnv(t))

	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, ":8080", cfg.HTTPAddr)
	assert.Equal(t, ":9090", cfg.MetricsAddr)
	assert.Equal(t, _marker, cfg.DB.Password.Value())
}

func TestLoad_ReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()

	_, err := config.Load(map[string]string{"LOG_LEVEL": "loud", "METRICS_ADDR": ":8080"})

	require.Error(t, err)
	for _, want := range []string{"DATABASE_URL", "DATABASE_PASSWORD_FILE", "LOG_LEVEL", "METRICS_ADDR"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestLoad_RejectsPasswordInDatabaseURL(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["DATABASE_URL"] = "postgres://app:" + _marker + "@db/craftpulse"

	_, err := config.Load(env)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_PASSWORD_FILE")
	assert.NotContains(t, err.Error(), _marker)
}

func TestSecret_NeverFormatted(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(validEnv(t))
	require.NoError(t, err)
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)

	for _, out := range []string{
		fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg), string(encoded),
	} {
		assert.NotContains(t, out, _marker)
	}
}
