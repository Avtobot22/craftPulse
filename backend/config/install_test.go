package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/config"
)

func installEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"SITE_URL":                               "https://craftpulse.example",
		"INSTALL_ORGANIZATIONS":                  "demo,second-org",
		"RUNNER_NETWORK":                         "10.20.0.0/16,192.0.2.10/32",
		"RELEASE_IMAGES":                         "craftpulse-server=sha256:aa,frontend=sha256:bb",
		"ORG_DEMO_NAME":                          "Demo",
		"ORG_DEMO_OIDC_ISSUER":                   "https://gitlab.example",
		"ORG_DEMO_OIDC_CLIENT_ID":                "demo-client",
		"ORG_DEMO_OIDC_CLIENT_SECRET_FILE":       secretFile(t, _marker+"\n"),
		"ORG_DEMO_ADMIN_SUBJECT":                 "17",
		"ORG_SECOND_ORG_NAME":                    "Second",
		"ORG_SECOND_ORG_OIDC_ISSUER":             "https://gitlab.example",
		"ORG_SECOND_ORG_OIDC_CLIENT_ID":          "second-client",
		"ORG_SECOND_ORG_OIDC_CLIENT_SECRET_FILE": secretFile(t, "second-"+_marker),
		"ORG_SECOND_ORG_ADMIN_SUBJECT":           "42",
	}
}

func TestLoadInstall(t *testing.T) {
	t.Parallel()

	in, err := config.LoadInstall(installEnv(t))

	require.NoError(t, err)
	require.Len(t, in.Organizations, 2)
	demo := in.Organizations[0]
	assert.Equal(t, "demo", demo.Slug)
	assert.Equal(t, "https://gitlab.example", demo.Issuer)
	assert.Equal(t, _marker, demo.ClientSecret.Value())
	assert.Equal(t, "second-org", in.Organizations[1].Slug)
	assert.Equal(t, "42", in.Organizations[1].AdminSubject)
	assert.Equal(t, []string{"10.20.0.0/16", "192.0.2.10/32"}, in.RunnerNetwork)
	assert.Equal(t, map[string]string{"craftpulse-server": "sha256:aa", "frontend": "sha256:bb"}, in.Images)
	assert.ElementsMatch(t, []string{_marker, "second-" + _marker}, in.Secrets())
}

func TestLoadInstall_ListsEveryMissingParameter(t *testing.T) {
	t.Parallel()

	_, err := config.LoadInstall(map[string]string{"INSTALL_ORGANIZATIONS": "demo"})

	require.Error(t, err)
	for _, want := range []string{
		"SITE_URL", "ORG_DEMO_NAME", "ORG_DEMO_OIDC_ISSUER", "ORG_DEMO_OIDC_CLIENT_ID",
		"ORG_DEMO_OIDC_CLIENT_SECRET_FILE", "ORG_DEMO_ADMIN_SUBJECT",
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestLoadInstall_RejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for name, change := range map[string]map[string]string{
		"slug":           {"INSTALL_ORGANIZATIONS": "Demo_Org"},
		"repeated slug":  {"INSTALL_ORGANIZATIONS": "demo,demo"},
		"http issuer":    {"ORG_DEMO_OIDC_ISSUER": "http://gitlab.example"},
		"site":           {"SITE_URL": "craftpulse.example"},
		"runner network": {"RUNNER_NETWORK": "10.20.0.0"},
	} {
		env := installEnv(t)
		for k, v := range change {
			env[k] = v
		}
		_, err := config.LoadInstall(env)
		require.Error(t, err, name)
		assert.NotContains(t, err.Error(), _marker, name)
	}
}
