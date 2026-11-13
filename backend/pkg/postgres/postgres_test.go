package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestRetry(t *testing.T) {
	tests := []struct {
		name      string
		giveErr   error
		wantCalls int
	}{
		{name: "success", giveErr: nil, wantCalls: 1},
		{name: "lock timeout", giveErr: &pgconn.PgError{Code: pgerrcode.LockNotAvailable}, wantCalls: _retries + 1},
		{name: "deadlock", giveErr: &pgconn.PgError{Code: pgerrcode.DeadlockDetected}, wantCalls: _retries + 1},
		{name: "serialization failure", giveErr: &pgconn.PgError{Code: pgerrcode.SerializationFailure}, wantCalls: _retries + 1},
		{name: "unique violation", giveErr: &pgconn.PgError{Code: pgerrcode.UniqueViolation}, wantCalls: 1},
		{name: "other error", giveErr: errors.New("boom"), wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			err := retry(t.Context(), func() error {
				calls++
				return tt.giveErr
			})
			assert.Equal(t, tt.wantCalls, calls)
			assert.Equal(t, tt.giveErr, err)
		})
	}
}

func TestRetry_StopsAfterSuccess(t *testing.T) {
	calls := 0
	err := retry(t.Context(), func() error {
		calls++
		if calls < 3 {
			return &pgconn.PgError{Code: pgerrcode.LockNotAvailable}
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetry_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	err := retry(ctx, func() error {
		calls++
		return &pgconn.PgError{Code: pgerrcode.DeadlockDetected}
	})
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}
