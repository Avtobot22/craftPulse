package config_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/config"
)

func adaptersEnv(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jira_cloud"), []byte("jira-key\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gitlab"), []byte("gitlab-key"), 0o600))
	return map[string]string{
		"BUILTIN_ADAPTERS":         "jira_cloud=https://jiracloud.adapters.internal:8443,gitlab=https://gitlab.adapters.internal:8443",
		"ADAPTER_SERVICE_KEYS_DIR": dir,
		"ADAPTER_CA_FILE":          "/run/secrets/adapter-ca.crt",
		"INSTALLATION_NETWORKS":    "10.1.0.0/16",
	}
}

func TestLoadAdapters(t *testing.T) {
	t.Parallel()
	a, err := config.LoadAdapters(adaptersEnv(t))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"jira_cloud": "https://jiracloud.adapters.internal:8443",
		"gitlab":     "https://gitlab.adapters.internal:8443",
	}, a.Builtin)
	assert.Equal(t, config.Secret("jira-key"), a.ServiceKeys["jira_cloud"], "the trailing newline of a secret file is dropped")
	assert.Equal(t, []string{"gitlab-key", "jira-key"}, a.Secrets())
	assert.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.1.0.0/16")}, a.InstallationNetworks)

	none, err := config.LoadAdapters(map[string]string{})
	require.NoError(t, err, "an installation may have only adapters of clients")
	assert.Empty(t, none.Builtin)

	for name, change := range map[string]map[string]string{
		"plain http":                {"BUILTIN_ADAPTERS": "jira_cloud=http://jiracloud.adapters.internal:8080"},
		"no service key file":       {"BUILTIN_ADAPTERS": "jenkins=https://jenkins.adapters.internal:8443"},
		"not a provider":            {"BUILTIN_ADAPTERS": "Jira Cloud=https://jiracloud.adapters.internal:8443"},
		"no internal CA":            {"ADAPTER_CA_FILE": ""},
		"no keys directory":         {"ADAPTER_SERVICE_KEYS_DIR": ""},
		"private without own scope": {"CLIENT_ADAPTER_PRIVATE_NETWORKS": "10.0.0.0/8", "INSTALLATION_NETWORKS": ""},
	} {
		env := adaptersEnv(t)
		for k, v := range change {
			env[k] = v
		}
		_, err := config.LoadAdapters(env)
		assert.Error(t, err, name)
	}
}
