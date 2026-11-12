package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// Keys that would carry what TZ 13.8 keeps out of the journal.
var _forbiddenKeys = map[string]struct{}{
	"token": {}, "access_token": {}, "refresh_token": {}, "id_token": {}, "secret": {}, "client_secret": {},
	"password": {}, "credentials": {}, "state": {}, "nonce": {}, "code": {}, "cookie": {}, "session_id": {},
}

type UseCase struct {
	repo  repo.AuditRepo
	tx    repo.Transactor
	clock clock.Clock
}

func New(r repo.AuditRepo, tx repo.Transactor, c clock.Clock) *UseCase {
	return &UseCase{repo: r, tx: tx, clock: c}
}

// Record writes the event in the caller's transaction: if it fails, the caller's change is rolled back too.
func (u *UseCase) Record(ctx context.Context, e entity.AuditEvent) error {
	if err := validate(e.Details); err != nil {
		return err
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = u.clock.Now()
	}
	return u.repo.InsertEvent(ctx, e)
}

// RecordSeparately commits the event on its own, so a denied access or a failed login stays recorded
// when the request itself is rolled back.
func (u *UseCase) RecordSeparately(ctx context.Context, e entity.AuditEvent) error {
	return u.tx.InOrgTx(ctx, e.OrganizationID, func(ctx context.Context) error { return u.Record(ctx, e) })
}

func validate(details map[string]any) error {
	for k := range details {
		if _, bad := _forbiddenKeys[k]; bad {
			return fmt.Errorf("%w: %q", entity.ErrAuditForbiddenDetail, k)
		}
	}
	if len(details) == 0 {
		return nil
	}
	b, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("audit: encode details: %w", err)
	}
	if len(b) > entity.AuditDetailsLimit {
		return entity.ErrAuditDetailsTooLarge
	}
	return nil
}
