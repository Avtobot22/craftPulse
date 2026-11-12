package cli

import (
	"context"
	"errors"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

var ErrCheckFailed = errors.New("installation check failed")

// Install applies the installation parameters, runs the check and prints the protocol through the
// logger, which masks secret values.
func Install(ctx context.Context, uc usecase.Installation, params entity.InstallParams, log zerolog.Logger) error {
	applied, err := uc.Apply(ctx, params)
	for _, a := range applied {
		log.Info().Str("organization", a.Slug).Str("organization_id", a.ID.String()).Bool("created", a.Created).
			Msg("organization")
	}
	if err != nil {
		return err
	}

	p, err := uc.Check(ctx, params)
	for _, c := range p.Checks {
		ev := log.Info()
		if c.Outcome == entity.CheckFailed {
			ev = log.Error()
		}
		ev.Str("check", string(c.Kind)).Str("outcome", string(c.Outcome)).Str("reason", c.Reason).
			Str("detail", c.Detail).Msg("installation check")
	}
	if err != nil {
		return err
	}
	log.Info().Str("protocol_id", p.ID.String()).Str("release_sha", p.ReleaseSHA).Str("outcome", string(p.Outcome)).
		Msg("installation protocol")
	if p.Outcome != entity.ProtocolPassed {
		return ErrCheckFailed
	}
	return nil
}
