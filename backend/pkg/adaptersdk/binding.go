package adaptersdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// SourceBinding pairs the address read by Probe with the verified identity of the source.
type SourceBinding struct {
	BaseURL     string
	InstanceKey string
	Version     string
	Edition     string
}

// BindSource saves a successful probe against the collect credential it actually used.
func (c Call) BindSource(ctx context.Context, usedCredential string, binding SourceBinding) error {
	return c.store.inOrg(ctx, c.Organization, func(ctx context.Context, db postgres.DBTX) error {
		current, _, err := c.store.collectBinding(ctx, db, c.Organization, c.Connection, true)
		if err != nil {
			return err
		}
		if current != usedCredential {
			return NewError(AdapterErrorCodeAuth, "collect credentials changed during probe")
		}
		_, err = db.Exec(ctx, `UPDATE adapter.credential SET source_binding = $2 WHERE connection_id = $1 AND purpose = 'collect'`,
			c.Connection, binding)
		if err != nil {
			return fmt.Errorf("adaptersdk: bind source: %w", err)
		}
		return nil
	})
}

// BoundSource reads the probed source and the collect credential in the same database snapshot.
func (c Call) BoundSource(ctx context.Context) (SourceBinding, string, error) {
	var binding *SourceBinding
	var secret string
	err := c.store.inOrg(ctx, c.Organization, func(ctx context.Context, db postgres.DBTX) error {
		var err error
		secret, binding, err = c.store.collectBinding(ctx, db, c.Organization, c.Connection, false)
		return err
	})
	if err != nil {
		return SourceBinding{}, "", err
	}
	if binding == nil {
		return SourceBinding{}, "", NewError(AdapterErrorCodeAuth, "source must be probed with the current credentials")
	}
	return *binding, secret, nil
}

func (s *Store) collectBinding(ctx context.Context, db postgres.DBTX, org, conn uuid.UUID, lock bool) (string, *SourceBinding, error) {
	query := `SELECT ciphertext, nonce, source_binding FROM adapter.credential WHERE connection_id = $1 AND purpose = 'collect'`
	if lock {
		query += " FOR UPDATE"
	}
	var ciphertext, nonce []byte
	var binding *SourceBinding
	err := db.QueryRow(ctx, query, conn).Scan(&ciphertext, &nonce, &binding)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, NewError(AdapterErrorCodeAuth, "credentials collect are not set")
	}
	if err != nil {
		return "", nil, fmt.Errorf("adaptersdk: read bound source: %w", err)
	}
	key, err := s.dataKey(ctx, db, org, false)
	if err != nil {
		return "", nil, err
	}
	secret, err := s.open(key, ciphertext, nonce, credentialAAD(org, conn, CredentialsInputPurposeCollect))
	return secret, binding, err
}
