package config

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog"
)

// Adapter is what every adapter process needs, `migrate` included: its own database and master key.
type Adapter struct {
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info"`
	DB         DB
	MasterKeys MasterKeys `envPrefix:"ADAPTER_"`
}

// AdapterServer is what a serving adapter needs on top of Adapter (TZ 13.6, 13.7). Installation networks
// are always refused to the source client; private source networks are allowed only in an in-contour
// installation, and then the installation networks must be listed.
type AdapterServer struct {
	HTTPAddr              string         `env:"HTTP_ADDR" envDefault:":8443"`
	TLSCertFile           string         `env:"TLS_CERT_FILE,required,notEmpty"`
	TLSKeyFile            string         `env:"TLS_KEY_FILE,required,notEmpty"`
	ServiceKey            Secret         `env:"ADAPTER_SERVICE_KEY_FILE,file,required,notEmpty"`
	PreviousServiceKey    Secret         `env:"ADAPTER_PREVIOUS_SERVICE_KEY_FILE,file"`
	SourceCAFile          string         `env:"SOURCE_CA_FILE"`
	InstallationNetworks  []netip.Prefix `env:"INSTALLATION_NETWORKS"`
	SourcePrivateNetworks []netip.Prefix `env:"SOURCE_PRIVATE_NETWORKS"`
	SpoolDir              string         `env:"SPOOL_DIR"`
}

func (a Adapter) Secrets() []string {
	return []string{a.DB.Password.Value(), a.MasterKeys.Current.Value(), a.MasterKeys.Previous.Value()}
}

func (s AdapterServer) Secrets() []string {
	return []string{s.ServiceKey.Value(), s.PreviousServiceKey.Value()}
}

func LoadAdapter(environ map[string]string) (Adapter, error) {
	var a Adapter
	errs := []error{env.ParseWithOptions(&a, env.Options{Environment: environ})}
	if _, err := zerolog.ParseLevel(a.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if a.DB.MaxConns < 1 {
		errs = append(errs, errors.New("DATABASE_MAX_CONNS must be positive"))
	}
	if a.DB.URL != "" {
		errs = append(errs, validateDatabaseURL(a.DB.URL))
	}
	errs = append(errs, a.MasterKeys.validate("ADAPTER_"))
	if err := errors.Join(errs...); err != nil {
		return Adapter{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return a, nil
}

func LoadAdapterServer(environ map[string]string) (AdapterServer, error) {
	var s AdapterServer
	errs := []error{env.ParseWithOptions(&s, env.Options{Environment: environ})}
	if s.PreviousServiceKey != "" && s.PreviousServiceKey == s.ServiceKey {
		errs = append(errs, errors.New("ADAPTER_PREVIOUS_SERVICE_KEY_FILE must differ from ADAPTER_SERVICE_KEY_FILE"))
	}
	if len(s.SourcePrivateNetworks) > 0 && len(s.InstallationNetworks) == 0 {
		errs = append(errs, errors.New("INSTALLATION_NETWORKS is required when SOURCE_PRIVATE_NETWORKS is set"))
	}
	if err := errors.Join(errs...); err != nil {
		return AdapterServer{}, fmt.Errorf("invalid adapter server: %w", err)
	}
	return s, nil
}
