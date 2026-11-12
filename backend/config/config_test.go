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
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func validEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"DATABASE_URL":                        "postgres://app@db:5432/craftpulse",
		"DATABASE_PASSWORD_FILE":              secretFile(t, _marker+"\n"),
		"OBJECT_STORE_ENDPOINT":               "http://seaweedfs:8333",
		"OBJECT_STORE_BUCKET":                 "craftpulse",
		"OBJECT_STORE_ACCESS_KEY_ID":          "craftpulse",
		"OBJECT_STORE_SECRET_ACCESS_KEY_FILE": secretFile(t, "s3-"+_marker),
		"CORE_MASTER_KEY_VERSION":             "2",
		"CORE_MASTER_KEY_FILE":                secretFile(t, _masterKey+"\n"),
	}
}

// base64 of 32 bytes.
const _masterKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

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

	_, err := config.Load(map[string]string{"LOG_LEVEL": "loud", "METRICS_ADDR": ":8080", "DATABASE_MAX_CONNS": "0"})

	require.Error(t, err)
	for _, want := range []string{
		"DATABASE_URL", "DATABASE_PASSWORD_FILE", "LOG_LEVEL", "METRICS_ADDR", "DATABASE_MAX_CONNS",
		"OBJECT_STORE_ENDPOINT", "OBJECT_STORE_SECRET_ACCESS_KEY_FILE", "CORE_MASTER_KEY_FILE",
	} {
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

func TestLoad_MasterKeys(t *testing.T) {
	t.Parallel()

	env := validEnv(t)
	env["CORE_PREVIOUS_MASTER_KEY_VERSION"] = "1"
	env["CORE_PREVIOUS_MASTER_KEY_FILE"] = secretFile(t, _masterKey)
	cfg, err := config.Load(env)
	require.NoError(t, err)
	keys, err := cfg.MasterKeys.Decode()
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.Equal(t, []uint32{2, 1}, []uint32{keys[0].Version, keys[1].Version})
	assert.Len(t, keys[0].Key, 32)
	assert.Contains(t, cfg.Secrets(), _masterKey)

	for name, change := range map[string]map[string]string{
		"short key":          {"CORE_MASTER_KEY_FILE": secretFile(t, "c2hvcnQ=")},
		"previous unversion": {"CORE_PREVIOUS_MASTER_KEY_FILE": secretFile(t, _masterKey)},
		"same version":       {"CORE_PREVIOUS_MASTER_KEY_FILE": secretFile(t, _masterKey), "CORE_PREVIOUS_MASTER_KEY_VERSION": "2"},
	} {
		env := validEnv(t)
		for k, v := range change {
			env[k] = v
		}
		_, err := config.Load(env)
		require.Error(t, err, name)
		assert.NotContains(t, err.Error(), _masterKey, name)
	}
}
