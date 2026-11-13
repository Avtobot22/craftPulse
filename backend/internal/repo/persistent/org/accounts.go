package org

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// EnsureSourceAccounts writes the accounts of an instance: a described one gets its login, name, email and
// proposed kind unless it was anonymized, a bare reference only appears. It returns the ID of every
// account by its native ID.
func (r *Repo) EnsureSourceAccounts(ctx context.Context, orgID, instanceID entity.ID, accounts []entity.SourceAccount,
	at time.Time,
) (map[string]entity.ID, error) {
	q := New(postgres.Conn(ctx, r.pool))
	org, inst := uuid.UUID(orgID), uuid.UUID(instanceID)
	var described, referred DescribeSourceAccountsParams
	for _, a := range accounts {
		if !a.Described {
			referred.NativeIds = append(referred.NativeIds, a.NativeID)
			continue
		}
		described.NativeIds = append(described.NativeIds, a.NativeID)
		described.Logins = append(described.Logins, a.Login)
		described.DisplayNames = append(described.DisplayNames, a.DisplayName)
		described.Emails = append(described.Emails, a.Email)
		described.Kinds = append(described.Kinds, string(a.Kind))
	}
	if len(described.NativeIds) > 0 {
		described.OrganizationID, described.InstanceID, described.SeenAt = org, inst, at
		if err := q.DescribeSourceAccounts(ctx, described); err != nil {
			return nil, fmt.Errorf("org: describe source accounts: %w", err)
		}
	}
	if len(referred.NativeIds) > 0 {
		err := q.ReferSourceAccounts(ctx, ReferSourceAccountsParams{OrganizationID: org, InstanceID: inst, NativeIds: referred.NativeIds, SeenAt: at})
		if err != nil {
			return nil, fmt.Errorf("org: refer source accounts: %w", err)
		}
	}
	natives := slices.Concat(described.NativeIds, referred.NativeIds)
	rows, err := q.SourceAccountIDs(ctx, SourceAccountIDsParams{OrganizationID: org, InstanceID: inst, NativeIds: natives})
	if err != nil {
		return nil, fmt.Errorf("org: source account IDs: %w", err)
	}
	out := make(map[string]entity.ID, len(rows))
	for _, row := range rows {
		out[row.NativeAccountID] = entity.ID(row.ID)
	}
	return out, nil
}
