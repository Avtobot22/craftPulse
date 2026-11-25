package connect

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestGitLabConnectionCredentials(t *testing.T) {
	t.Parallel()
	c := entity.Connection{Provider: entity.ProviderGitLab, AdapterKind: entity.AdapterBuiltin}
	in, err := connectionCredentials(c, map[string]entity.Secret{
		"read_token": "reader", "write_token": "writer", "webhook_secret": "hook",
	})
	require.NoError(t, err)
	assert.Equal(t, []entity.CredentialsInput{
		{Purpose: "collect", Secret: `{"token":"reader"}`},
		{Purpose: "write_bot", Secret: "writer"},
		{Purpose: "webhook_secret", Secret: "hook"},
	}, in)

	in, err = connectionCredentials(c, map[string]entity.Secret{"webhook_secret": "rotated"})
	require.NoError(t, err)
	assert.Equal(t, []entity.CredentialsInput{{Purpose: "webhook_secret", Secret: "rotated"}}, in)
}

func TestGitLabConnectionCredentialsSkipsBlankOptionalFields(t *testing.T) {
	t.Parallel()
	c := entity.Connection{Provider: entity.ProviderGitLab, AdapterKind: entity.AdapterBuiltin}
	in, err := connectionCredentials(c, map[string]entity.Secret{
		"read_token": "reader", "write_token": "", "webhook_secret": "",
	})
	require.NoError(t, err)
	assert.Equal(t, []entity.CredentialsInput{
		{Purpose: "collect", Secret: `{"token":"reader"}`},
	}, in)
}

func TestWriteBotAuthDoesNotSettleCollectorAccess(t *testing.T) {
	t.Parallel()
	u := New(Deps{})
	c := entity.Connection{AdapterKind: entity.AdapterBuiltin, ContractVersion: ContractVersions[0]}
	refusal := &entity.AdapterError{Class: entity.CallSource, Code: entity.SourceAuth}
	for _, op := range []entity.AdapterOp{entity.OpFindPublished, entity.OpPublish} {
		err := u.invoke(t.Context(), c, op, func(context.Context, entity.AdapterTarget) error {
			return refusal
		})
		require.ErrorIs(t, err, refusal)
	}
}
