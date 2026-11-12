package install

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/migrations"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// sqlc does not know the system catalogs without a live database, so this query is written by hand.
const _databaseFacts = `SELECT d.datlocprovider::text, COALESCE(d.datlocale, d.datcollate)::text,
	EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'btree_gist')
FROM pg_database d WHERE d.datname = current_database()`

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) CreateProtocol(ctx context.Context, startedAt time.Time, releaseSHA string, manifest []byte) (entity.ID, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).CreateProtocol(ctx, CreateProtocolParams{
		StartedAt: startedAt, ReleaseSha: releaseSHA, Manifest: manifest,
	})
	if err != nil {
		return entity.ID{}, fmt.Errorf("install: create protocol: %w", err)
	}
	return entity.ID(id), nil
}

func (r *Repo) AddCheckResult(ctx context.Context, protocolID entity.ID, c entity.CheckResult) error {
	err := New(postgres.Conn(ctx, r.pool)).InsertCheckResult(ctx, InsertCheckResultParams{
		ProtocolID: uuid.UUID(protocolID),
		CheckKind:  string(c.Kind),
		Outcome:    string(c.Outcome),
		ReasonCode: pgtype.Text{String: c.Reason, Valid: c.Reason != ""},
		Detail:     pgtype.Text{String: c.Detail, Valid: c.Detail != ""},
	})
	if err != nil {
		return fmt.Errorf("install: add check result %s: %w", c.Kind, err)
	}
	return nil
}

func (r *Repo) FinishProtocol(ctx context.Context, protocolID entity.ID, finishedAt time.Time, outcome entity.ProtocolOutcome) error {
	err := New(postgres.Conn(ctx, r.pool)).FinishProtocol(ctx, FinishProtocolParams{
		ID:         uuid.UUID(protocolID),
		FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
		Outcome:    string(outcome),
	})
	if err != nil {
		return fmt.Errorf("install: finish protocol: %w", err)
	}
	return nil
}

func (r *Repo) DatabaseFacts(ctx context.Context) (entity.DatabaseFacts, error) {
	var f entity.DatabaseFacts
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, _databaseFacts).Scan(&f.LocaleProvider, &f.Locale, &f.HasBtreeGist)
	if err != nil {
		return entity.DatabaseFacts{}, fmt.Errorf("install: read database settings: %w", err)
	}
	if f.SchemaVersion, err = migrations.Current(ctx, r.pool); err != nil {
		return entity.DatabaseFacts{}, err
	}
	f.ExpectedSchema = migrations.Latest()
	return f, nil
}

// LatestProtocol returns the protocol of the last installation check with the result of each check.
func (r *Repo) LatestProtocol(ctx context.Context) (entity.Protocol, bool, error) {
	q := New(postgres.Conn(ctx, r.pool))
	row, err := q.GetLatestProtocol(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Protocol{}, false, nil
	}
	if err != nil {
		return entity.Protocol{}, false, fmt.Errorf("install: get latest protocol: %w", err)
	}
	checks, err := q.ListCheckResults(ctx, row.ID)
	if err != nil {
		return entity.Protocol{}, false, fmt.Errorf("install: list check results: %w", err)
	}
	p := entity.Protocol{
		ID:         entity.ID(row.ID),
		StartedAt:  row.StartedAt,
		FinishedAt: row.FinishedAt.Time,
		ReleaseSHA: row.ReleaseSha,
		Outcome:    entity.ProtocolOutcome(row.Outcome),
		Checks:     make([]entity.CheckResult, len(checks)),
	}
	for i, c := range checks {
		p.Checks[i] = entity.CheckResult{
			Kind:    entity.CheckKind(c.CheckKind),
			Outcome: entity.CheckOutcome(c.Outcome),
			Reason:  c.ReasonCode.String,
			Detail:  c.Detail.String,
		}
	}
	return p, true, nil
}
