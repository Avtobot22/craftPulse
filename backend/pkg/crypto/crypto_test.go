package crypto_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestKeyring_SealOpen(t *testing.T) {
	t.Parallel()
	k, err := crypto.NewKeyring(crypto.MasterKey{Version: 1, Key: key(1)})
	require.NoError(t, err)
	dk := k.NewDataKey()
	aad := []byte("org-a")

	ct, err := k.Seal(dk, []byte("token"), aad)
	require.NoError(t, err)
	assert.NotContains(t, string(ct), "token")

	got, err := k.Open(dk, ct, aad)
	require.NoError(t, err)
	assert.Equal(t, "token", string(got))

	tampered := bytes.Clone(ct)
	tampered[len(tampered)-1] ^= 1
	_, err = k.Open(dk, tampered, aad)
	require.ErrorIs(t, err, crypto.ErrDecrypt)
	_, err = k.Open(dk, ct, []byte("org-b"))
	require.ErrorIs(t, err, crypto.ErrDecrypt, "ciphertext is bound to its context")
}

func TestKeyring_RotationRewrapsOnlyDataKey(t *testing.T) {
	t.Parallel()
	before, err := crypto.NewKeyring(crypto.MasterKey{Version: 1, Key: key(1)})
	require.NoError(t, err)
	dk := before.NewDataKey()
	ct, err := before.Seal(dk, []byte("token"), nil)
	require.NoError(t, err)

	after, err := crypto.NewKeyring(crypto.MasterKey{Version: 2, Key: key(2)}, crypto.MasterKey{Version: 1, Key: key(1)})
	require.NoError(t, err)
	rewrapped, err := after.Rewrap(dk)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), rewrapped.MasterVersion)

	only2, err := crypto.NewKeyring(crypto.MasterKey{Version: 2, Key: key(2)})
	require.NoError(t, err)
	got, err := only2.Open(rewrapped, ct, nil)
	require.NoError(t, err, "data encrypted before rotation opens with the rewrapped key")
	assert.Equal(t, "token", string(got))

	_, err = only2.Open(dk, ct, nil)
	require.ErrorIs(t, err, crypto.ErrUnknownMasterKey)
}

func TestNewKeyring_RejectsShortKey(t *testing.T) {
	t.Parallel()
	_, err := crypto.NewKeyring(crypto.MasterKey{Version: 1, Key: []byte("short")})
	assert.Error(t, err)
}

func TestKeyring_Envelope(t *testing.T) {
	t.Parallel()
	k, err := crypto.NewKeyring(crypto.MasterKey{Version: 2, Key: key(2)}, crypto.MasterKey{Version: 1, Key: key(1)})
	require.NoError(t, err)
	aad := []byte("org-a")

	blob, version, err := k.SealEnvelope([]byte("client-secret"), aad)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), version)
	assert.NotContains(t, string(blob), "client-secret")

	got, err := k.OpenEnvelope(blob, version, aad)
	require.NoError(t, err)
	assert.Equal(t, "client-secret", string(got))

	for name, open := range map[string]func() ([]byte, error){
		"other context": func() ([]byte, error) { return k.OpenEnvelope(blob, version, []byte("org-b")) },
		"modified blob": func() ([]byte, error) {
			b := bytes.Clone(blob)
			b[len(b)-1] ^= 1
			return k.OpenEnvelope(b, version, aad)
		},
		"truncated blob":    func() ([]byte, error) { return k.OpenEnvelope(blob[:1], version, aad) },
		"wrong key version": func() ([]byte, error) { return k.OpenEnvelope(blob, 1, aad) },
	} {
		_, err := open()
		assert.Error(t, err, name)
	}

	old, err := crypto.NewKeyring(crypto.MasterKey{Version: 1, Key: key(1)})
	require.NoError(t, err)
	oldBlob, oldVersion, err := old.SealEnvelope([]byte("client-secret"), aad)
	require.NoError(t, err)
	got, err = k.OpenEnvelope(oldBlob, oldVersion, aad)
	require.NoError(t, err, "a blob wrapped by the previous master key still opens")
	assert.Equal(t, "client-secret", string(got))
}
