package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog"
)

const _redacted = "[REDACTED]"

type Secret string

func (s Secret) Value() string { return string(s) }

func (Secret) String() string { return _redacted }

func (Secret) GoString() string { return _redacted }

func (Secret) MarshalText() ([]byte, error) { return []byte(_redacted), nil }

// UnmarshalText drops the trailing newline that editors and secret tooling add to files.
func (s *Secret) UnmarshalText(b []byte) error {
	*s = Secret(strings.TrimRight(string(b), "\r\n"))
	return nil
}

type Config struct {
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	MetricsAddr string `env:"METRICS_ADDR" envDefault:":9090"`
	DB          DB
}

type DB struct {
	URL      string `env:"DATABASE_URL,required,notEmpty"`
	Password Secret `env:"DATABASE_PASSWORD_FILE,file,required,notEmpty"`
	MaxConns int32  `env:"DATABASE_MAX_CONNS" envDefault:"10"`
}

func (c Config) Secrets() []string {
	return []string{c.DB.Password.Value()}
}

func Load(environ map[string]string) (Config, error) {
	var cfg Config
	errs := []error{env.ParseWithOptions(&cfg, env.Options{Environment: environ})}
	if _, err := zerolog.ParseLevel(cfg.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if cfg.HTTPAddr == cfg.MetricsAddr {
		errs = append(errs, errors.New("METRICS_ADDR must differ from HTTP_ADDR"))
	}
	if cfg.DB.MaxConns < 1 {
		errs = append(errs, errors.New("DATABASE_MAX_CONNS must be positive"))
	}
	if cfg.DB.URL != "" {
		errs = append(errs, validateDatabaseURL(cfg.DB.URL))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

func validateDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return errors.New("DATABASE_URL must be a postgres:// URL")
	}
	if _, has := u.User.Password(); has || u.Query().Has("password") {
		return errors.New("DATABASE_URL must not contain a password, use DATABASE_PASSWORD_FILE")
	}
	return nil
}
