package config

import (
	"encoding/base64"
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
	ObjectStore ObjectStore
	MasterKeys  MasterKeys `envPrefix:"CORE_"`
}

type DB struct {
	URL      string `env:"DATABASE_URL,required,notEmpty"`
	Password Secret `env:"DATABASE_PASSWORD_FILE,file,required,notEmpty"`
	MaxConns int32  `env:"DATABASE_MAX_CONNS" envDefault:"10"`
}

type ObjectStore struct {
	Endpoint        string `env:"OBJECT_STORE_ENDPOINT,required,notEmpty"`
	Region          string `env:"OBJECT_STORE_REGION" envDefault:"us-east-1"`
	Bucket          string `env:"OBJECT_STORE_BUCKET,required,notEmpty"`
	AccessKeyID     string `env:"OBJECT_STORE_ACCESS_KEY_ID,required,notEmpty"`
	SecretAccessKey Secret `env:"OBJECT_STORE_SECRET_ACCESS_KEY_FILE,file,required,notEmpty"`
}

// MasterKeys are the keys of pkg/crypto (TZ 14.2): files with base64 of 32 bytes, under the CORE_ prefix
// for the core and ADAPTER_ for an adapter. The previous key is set only during rotation, until data keys
// are rewrapped by the current one.
type MasterKeys struct {
	CurrentVersion  uint32 `env:"MASTER_KEY_VERSION,required"`
	Current         Secret `env:"MASTER_KEY_FILE,file,required,notEmpty"`
	PreviousVersion uint32 `env:"PREVIOUS_MASTER_KEY_VERSION"`
	Previous        Secret `env:"PREVIOUS_MASTER_KEY_FILE,file"`
}

type MasterKey struct {
	Version uint32
	Key     []byte
}

// Decode returns the current key first, then the previous one if it is set.
func (m MasterKeys) Decode() ([]MasterKey, error) {
	keys := []MasterKey{{Version: m.CurrentVersion}}
	if m.Previous != "" {
		keys = append(keys, MasterKey{Version: m.PreviousVersion})
	}
	var errs []error
	for i, s := range []Secret{m.Current, m.Previous}[:len(keys)] {
		key, err := base64.StdEncoding.DecodeString(s.Value())
		if err != nil || len(key) != 32 {
			errs = append(errs, fmt.Errorf("master key version %d must be base64 of 32 bytes", keys[i].Version))
		}
		keys[i].Key = key
	}
	return keys, errors.Join(errs...)
}

func (m MasterKeys) validate(prefix string) error {
	var errs []error
	if m.CurrentVersion == 0 {
		errs = append(errs, fmt.Errorf("%sMASTER_KEY_VERSION must be positive", prefix))
	}
	if (m.Previous == "") != (m.PreviousVersion == 0) {
		errs = append(errs, fmt.Errorf("%[1]sPREVIOUS_MASTER_KEY_FILE and %[1]sPREVIOUS_MASTER_KEY_VERSION are set together", prefix))
	}
	if m.Previous != "" && m.PreviousVersion == m.CurrentVersion {
		errs = append(errs, fmt.Errorf("%[1]sPREVIOUS_MASTER_KEY_VERSION must differ from %[1]sMASTER_KEY_VERSION", prefix))
	}
	if m.Current != "" {
		if _, err := m.Decode(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c Config) Secrets() []string {
	return []string{c.DB.Password.Value(), c.ObjectStore.SecretAccessKey.Value(), c.MasterKeys.Current.Value(), c.MasterKeys.Previous.Value()}
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
	if cfg.ObjectStore.Endpoint != "" {
		errs = append(errs, validateHTTPURL("OBJECT_STORE_ENDPOINT", cfg.ObjectStore.Endpoint))
	}
	errs = append(errs, cfg.MasterKeys.validate("CORE_"))
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

func validateHTTPURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return fmt.Errorf("%s must be an http(s) URL without credentials", name)
	}
	return nil
}

// Site is the address api serves (TZ 13.1, 13.2): the provider returns the browser to it, and a changing
// request with the session cookie must carry it as Origin.
type Site struct {
	URL string `env:"SITE_URL,required,notEmpty"`
}

func LoadSite(environ map[string]string) (Site, error) {
	var s Site
	if err := env.ParseWithOptions(&s, env.Options{Environment: environ}); err != nil {
		return Site{}, fmt.Errorf("invalid site: %w", err)
	}
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return Site{}, errors.New("invalid site: SITE_URL must be an http(s) origin without path, query and credentials")
	}
	s.URL = u.Scheme + "://" + u.Host
	return s, nil
}
