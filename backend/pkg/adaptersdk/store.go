package adaptersdk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

const _nonceSize = 12

var (
	ErrNoCredential       = errors.New("adaptersdk: credentials are not set")
	errMasterKeyNotLoaded = errors.New("adaptersdk: master key is not loaded")
)

// UserToken is the token a user granted for access checks under their own account (TZ 13.6).
type UserToken struct {
	AccountNativeID string
	Access          string
	Refresh         string
	ExpiresAt       *time.Time
}

// Store keeps credentials in the adapter database (TZ 12.5.9): each value is sealed by the data key of
// its organization, and that key by the adapter master key (TZ 11). Every query runs in a transaction
// of the organization, so row-level security applies to the adapter process.
type Store struct {
	pool *pgxpool.Pool
	keys *crypto.Keyring
	now  func() time.Time
}

func NewStore(pool *pgxpool.Pool, keys *crypto.Keyring) *Store {
	return &Store{pool: pool, keys: keys, now: time.Now}
}

// Ready reports whether the master key is loaded, for /readyz.
func (s *Store) Ready(context.Context) error {
	if s.keys == nil {
		return errMasterKeyNotLoaded
	}
	return nil
}

// PutCredential replaces the value of the purpose and keeps the former one until DropPrevious.
func (s *Store) PutCredential(ctx context.Context, org, conn uuid.UUID, purpose CredentialsInputPurpose, secret string) (time.Time, error) {
	at := s.stamp()
	err := s.inOrg(ctx, org, func(ctx context.Context, db postgres.DBTX) error {
		key, err := s.dataKey(ctx, db, org, true)
		if err != nil {
			return err
		}
		ciphertext, nonce, err := s.seal(key, secret, credentialAAD(org, conn, purpose))
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, `
INSERT INTO adapter.credential (organization_id, connection_id, purpose, ciphertext, nonce, replaced_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (connection_id, purpose) DO UPDATE SET
    previous_ciphertext = adapter.credential.ciphertext,
    previous_nonce = adapter.credential.nonce,
    ciphertext = EXCLUDED.ciphertext,
    nonce = EXCLUDED.nonce,
    replaced_at = EXCLUDED.replaced_at`, org, conn, string(purpose), ciphertext, nonce, at)
		if err != nil {
			return fmt.Errorf("adaptersdk: save credential: %w", err)
		}
		return nil
	})
	return at, err
}

// Credential returns the current value of the purpose or ErrNoCredential.
func (s *Store) Credential(ctx context.Context, org, conn uuid.UUID, purpose CredentialsInputPurpose) (string, error) {
	var secret string
	err := s.inOrg(ctx, org, func(ctx context.Context, db postgres.DBTX) error {
		var ciphertext, nonce []byte
		err := db.QueryRow(ctx, `SELECT ciphertext, nonce FROM adapter.credential WHERE connection_id = $1 AND purpose = $2`,
			conn, string(purpose)).Scan(&ciphertext, &nonce)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoCredential
		}
		if err != nil {
			return fmt.Errorf("adaptersdk: read credential: %w", err)
		}
		key, err := s.dataKey(ctx, db, org, false)
		if err != nil {
			return err
		}
		secret, err = s.open(key, ciphertext, nonce, credentialAAD(org, conn, purpose))
		return err
	})
	return secret, err
}

// DropPrevious removes the values kept since the last replacement, after a successful Probe.
func (s *Store) DropPrevious(ctx context.Context, org, conn uuid.UUID) error {
	return s.inOrg(ctx, org, func(ctx context.Context, db postgres.DBTX) error {
		_, err := db.Exec(ctx, `
UPDATE adapter.credential SET previous_ciphertext = NULL, previous_nonce = NULL
WHERE connection_id = $1 AND previous_ciphertext IS NOT NULL`, conn)
		if err != nil {
			return fmt.Errorf("adaptersdk: drop previous credentials: %w", err)
		}
		return nil
	})
}

// DeleteCredentials removes one purpose, the token of one user, all user tokens, or, with neither
// given, everything of the connection. Deleting what is absent is not an error.
func (s *Store) DeleteCredentials(ctx context.Context, org, conn uuid.UUID, purpose *CredentialsInputPurpose, user *uuid.UUID) error {
	all := purpose == nil && user == nil
	tokens := all || user != nil || *purpose == CredentialsInputPurposeUserToken
	return s.inOrg(ctx, org, func(ctx context.Context, db postgres.DBTX) error {
		if all || (purpose != nil && *purpose != CredentialsInputPurposeUserToken) {
			q, args := `DELETE FROM adapter.credential WHERE connection_id = $1`, []any{conn}
			if purpose != nil {
				q, args = q+` AND purpose = $2`, append(args, string(*purpose))
			}
			if _, err := db.Exec(ctx, q, args...); err != nil {
				return fmt.Errorf("adaptersdk: delete credentials: %w", err)
			}
		}
		if tokens {
			q, args := `DELETE FROM adapter.user_token WHERE connection_id = $1`, []any{conn}
			if user != nil {
				q, args = q+` AND user_id = $2`, append(args, *user)
			}
			if _, err := db.Exec(ctx, q, args...); err != nil {
				return fmt.Errorf("adaptersdk: delete user tokens: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) PutUserToken(ctx context.Context, org, conn, user uuid.UUID, tok UserToken) (time.Time, error) {
	at := s.stamp()
	err := s.inOrg(ctx, org, func(ctx context.Context, db postgres.DBTX) error {
		key, err := s.dataKey(ctx, db, org, true)
		if err != nil {
			return err
		}
		access, nonce, err := s.seal(key, tok.Access, userTokenAAD(org, conn, user, "access"))
		if err != nil {
			return err
		}
		var refresh, refreshNonce []byte
		if tok.Refresh != "" {
			if refresh, refreshNonce, err = s.seal(key, tok.Refresh, userTokenAAD(org, conn, user, "refresh")); err != nil {
				return err
			}
		}
		var account *string
		if tok.AccountNativeID != "" {
			account = &tok.AccountNativeID
		}
		_, err = db.Exec(ctx, `
INSERT INTO adapter.user_token (organization_id, connection_id, user_id, account_native_id, access_ciphertext, nonce,
    refresh_ciphertext, refresh_nonce, expires_at, replaced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (connection_id, user_id) DO UPDATE SET
    account_native_id = EXCLUDED.account_native_id,
    access_ciphertext = EXCLUDED.access_ciphertext,
    nonce = EXCLUDED.nonce,
    refresh_ciphertext = EXCLUDED.refresh_ciphertext,
    refresh_nonce = EXCLUDED.refresh_nonce,
    expires_at = EXCLUDED.expires_at,
    replaced_at = EXCLUDED.replaced_at`,
			org, conn, user, account, access, nonce, refresh, refreshNonce, tok.ExpiresAt, at)
		if err != nil {
			return fmt.Errorf("adaptersdk: save user token: %w", err)
		}
		return nil
	})
	return at, err
}

// UserToken returns the token of the user or ErrNoCredential.
func (s *Store) UserToken(ctx context.Context, org, conn, user uuid.UUID) (UserToken, error) {
	var tok UserToken
	err := s.inOrg(ctx, org, func(ctx context.Context, db postgres.DBTX) error {
		var (
			account                              *string
			access, nonce, refresh, refreshNonce []byte
		)
		err := db.QueryRow(ctx, `
SELECT account_native_id, access_ciphertext, nonce, refresh_ciphertext, refresh_nonce, expires_at
FROM adapter.user_token WHERE connection_id = $1 AND user_id = $2`, conn, user).
			Scan(&account, &access, &nonce, &refresh, &refreshNonce, &tok.ExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoCredential
		}
		if err != nil {
			return fmt.Errorf("adaptersdk: read user token: %w", err)
		}
		key, err := s.dataKey(ctx, db, org, false)
		if err != nil {
			return err
		}
		if account != nil {
			tok.AccountNativeID = *account
		}
		if tok.Access, err = s.open(key, access, nonce, userTokenAAD(org, conn, user, "access")); err != nil {
			return err
		}
		if refresh != nil {
			tok.Refresh, err = s.open(key, refresh, refreshNonce, userTokenAAD(org, conn, user, "refresh"))
		}
		return err
	})
	return tok, err
}

// RewrapKeys wraps every data key that is not under the current master key with it, so the previous
// master key can be removed. It reads all organizations and so runs under the owner role, from migrate.
func (s *Store) RewrapKeys(ctx context.Context) (int, error) {
	current := s.keys.CurrentVersion()
	n := 0
	err := postgres.InTx(ctx, s.pool, func(ctx context.Context) error {
		db := postgres.Conn(ctx, s.pool)
		rows, err := db.Query(ctx, `
SELECT organization_id, data_key_enc, master_key_version FROM adapter.org_key
WHERE master_key_version <> $1 FOR UPDATE`, int64(current))
		if err != nil {
			return fmt.Errorf("adaptersdk: read data keys: %w", err)
		}
		type stale struct {
			org uuid.UUID
			key crypto.WrappedKey
		}
		var keys []stale
		for rows.Next() {
			var (
				k       stale
				version int64
			)
			if err := rows.Scan(&k.org, &k.key.Ciphertext, &version); err != nil {
				rows.Close()
				return fmt.Errorf("adaptersdk: read data key: %w", err)
			}
			k.key.MasterVersion = uint32(version) //nolint:gosec // the column is a positive integer
			keys = append(keys, k)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("adaptersdk: read data keys: %w", err)
		}
		for _, k := range keys {
			if err := s.rewrap(ctx, db, k.org, k.key); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// dataKey returns the data key of the organization, creating it if asked, and rewraps it when it is
// still under a previous master key.
func (s *Store) dataKey(ctx context.Context, db postgres.DBTX, org uuid.UUID, create bool) (crypto.WrappedKey, error) {
	if s.keys == nil {
		return crypto.WrappedKey{}, errMasterKeyNotLoaded
	}
	key, err := readDataKey(ctx, db, org)
	if errors.Is(err, pgx.ErrNoRows) && create {
		fresh := s.keys.NewDataKey()
		if _, err := db.Exec(ctx, `
INSERT INTO adapter.org_key (organization_id, data_key_enc, master_key_version) VALUES ($1, $2, $3)
ON CONFLICT (organization_id) DO NOTHING`, org, fresh.Ciphertext, int64(fresh.MasterVersion)); err != nil {
			return crypto.WrappedKey{}, fmt.Errorf("adaptersdk: create data key: %w", err)
		}
		key, err = readDataKey(ctx, db, org)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return crypto.WrappedKey{}, ErrNoCredential
	}
	if err != nil {
		return crypto.WrappedKey{}, fmt.Errorf("adaptersdk: read data key: %w", err)
	}
	if key.MasterVersion != s.keys.CurrentVersion() {
		if err := s.rewrap(ctx, db, org, key); err != nil {
			return crypto.WrappedKey{}, err
		}
	}
	return key, nil
}

func readDataKey(ctx context.Context, db postgres.DBTX, org uuid.UUID) (crypto.WrappedKey, error) {
	var (
		key     crypto.WrappedKey
		version int64
	)
	err := db.QueryRow(ctx, `SELECT data_key_enc, master_key_version FROM adapter.org_key WHERE organization_id = $1`, org).
		Scan(&key.Ciphertext, &version)
	key.MasterVersion = uint32(version) //nolint:gosec // the column is a positive integer
	return key, err
}

func (s *Store) rewrap(ctx context.Context, db postgres.DBTX, org uuid.UUID, key crypto.WrappedKey) error {
	fresh, err := s.keys.Rewrap(key)
	if err != nil {
		return fmt.Errorf("adaptersdk: rewrap data key: %w", err)
	}
	_, err = db.Exec(ctx, `UPDATE adapter.org_key SET data_key_enc = $2, master_key_version = $3 WHERE organization_id = $1`,
		org, fresh.Ciphertext, int64(fresh.MasterVersion))
	if err != nil {
		return fmt.Errorf("adaptersdk: save rewrapped data key: %w", err)
	}
	return nil
}

func (s *Store) inOrg(ctx context.Context, org uuid.UUID, fn func(ctx context.Context, db postgres.DBTX) error) error {
	return postgres.InOrgTx(ctx, s.pool, org, func(ctx context.Context) error {
		return fn(ctx, postgres.Conn(ctx, s.pool))
	})
}

// seal returns the ciphertext and the nonce separately, as the adapter schema stores them.
func (s *Store) seal(key crypto.WrappedKey, plaintext string, aad []byte) (ciphertext, nonce []byte, err error) {
	sealed, err := s.keys.Seal(key, []byte(plaintext), aad)
	if err != nil {
		return nil, nil, fmt.Errorf("adaptersdk: seal credential: %w", err)
	}
	return sealed[_nonceSize:], sealed[:_nonceSize], nil
}

func (s *Store) open(key crypto.WrappedKey, ciphertext, nonce, aad []byte) (string, error) {
	plaintext, err := s.keys.Open(key, slices.Concat(nonce, ciphertext), aad)
	if err != nil {
		return "", fmt.Errorf("adaptersdk: open credential: %w", err)
	}
	return string(plaintext), nil
}

func (s *Store) stamp() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// The additional data binds a ciphertext to its row, so a value moved to another row does not open.
func credentialAAD(org, conn uuid.UUID, purpose CredentialsInputPurpose) []byte {
	return []byte("adapter.credential\x00" + org.String() + "\x00" + conn.String() + "\x00" + string(purpose))
}

func userTokenAAD(org, conn, user uuid.UUID, part string) []byte {
	return []byte("adapter.user_token\x00" + org.String() + "\x00" + conn.String() + "\x00" + user.String() + "\x00" + part)
}
