package config_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/config"
)

func adapterEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"DATABASE_URL":               "postgres://adapter_jiracloud@db:5432/adapter_jiracloud",
		"DATABASE_PASSWORD_FILE":     secretFile(t, _marker+"\n"),
		"ADAPTER_MASTER_KEY_VERSION": "1",
		"ADAPTER_MASTER_KEY_FILE":    secretFile(t, _masterKey+"\n"),
		"TLS_CERT_FILE":              "/run/secrets/tls.crt",
		"TLS_KEY_FILE":               "/run/secrets/tls.key",
		"ADAPTER_SERVICE_KEY_FILE":   secretFile(t, "service-key\n"),
	}
}

func TestLoadAdapter(t *testing.T) {
	t.Parallel()
	env := adapterEnv(t)
	a, err := config.LoadAdapter(env)
	require.NoError(t, err)
	keys, err := a.MasterKeys.Decode()
	require.NoError(t, err)
	assert.Equal(t, uint32(1), keys[0].Version)
	assert.Contains(t, a.Secrets(), _masterKey)

	env["CORE_MASTER_KEY_FILE"] = env["ADAPTER_MASTER_KEY_FILE"]
	delete(env, "ADAPTER_MASTER_KEY_FILE")
	_, err = config.LoadAdapter(env)
	assert.ErrorContains(t, err, "ADAPTER_MASTER_KEY_FILE", "an adapter reads only its own master key")
}

func TestLoadAdapterServer(t *testing.T) {
	t.Parallel()
	env := adapterEnv(t)
	env["INSTALLATION_NETWORKS"] = "10.1.0.0/16,fd00:1::/64"
	env["SOURCE_PRIVATE_NETWORKS"] = "10.0.0.0/8"
	s, err := config.LoadAdapterServer(env)
	require.NoError(t, err)
	assert.Equal(t, ":8443", s.HTTPAddr)
	assert.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.1.0.0/16"), netip.MustParsePrefix("fd00:1::/64")}, s.InstallationNetworks)
	assert.Equal(t, []string{"service-key", ""}, s.Secrets())

	for name, change := range map[string]map[string]string{
		"no service key":            {"ADAPTER_SERVICE_KEY_FILE": ""},
		"not a network":             {"INSTALLATION_NETWORKS": "10.1.0.1"},
		"private without own scope": {"INSTALLATION_NETWORKS": ""},
		"same previous key":         {"ADAPTER_PREVIOUS_SERVICE_KEY_FILE": env["ADAPTER_SERVICE_KEY_FILE"]},
	} {
		env := adapterEnv(t)
		env["INSTALLATION_NETWORKS"] = "10.1.0.0/16"
		env["SOURCE_PRIVATE_NETWORKS"] = "10.0.0.0/8"
		for k, v := range change {
			env[k] = v
		}
		_, err := config.LoadAdapterServer(env)
		assert.Error(t, err, name)
	}
}
