package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func New(w io.Writer, level, service string, secrets []string) (zerolog.Logger, error) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Nop(), fmt.Errorf("logger: parse level %q: %w", level, err)
	}
	return zerolog.New(newRedactor(w, secrets)).
		Level(lvl).
		With().
		Timestamp().
		Str("service", service).
		Logger(), nil
}

// Slog serves libraries that log through log/slog, such as River, with the same secret masking.
func Slog(w io.Writer, level slog.Level, service string, secrets []string) *slog.Logger {
	h := slog.NewJSONHandler(newRedactor(w, secrets), &slog.HandlerOptions{Level: level})
	return slog.New(h).With("service", service)
}

func Default(service string) zerolog.Logger {
	return zerolog.New(os.Stderr).With().Timestamp().Str("service", service).Logger()
}

type correlationKey struct{}

func WithCorrelation(ctx context.Context, log zerolog.Logger) context.Context {
	id := uuid.Must(uuid.NewV7())
	ctx = context.WithValue(ctx, correlationKey{}, id)
	return log.With().Str("correlation_id", id.String()).Logger().WithContext(ctx)
}

func CorrelationID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(correlationKey{}).(uuid.UUID)
	return id
}
