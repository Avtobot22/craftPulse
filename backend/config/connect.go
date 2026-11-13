package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/caarlos0/env/v11"
)

var _providerName = regexp.MustCompile(`^[a-z][a-z_]{1,31}$`)

// Adapters is how api and worker reach adapters (TZ 13.6, 13.7, 14.2). A built-in adapter is
// provider=https-root in BUILTIN_ADAPTERS, its service key is the file ADAPTER_SERVICE_KEYS_DIR/<provider>,
// and it is trusted only through ADAPTER_CA_FILE. Adapters of clients are refused in the installation
// networks; private networks are allowed to them only when listed.
type Adapters struct {
	Builtin               map[string]string `env:"BUILTIN_ADAPTERS" envKeyValSeparator:"="`
	ServiceKeysDir        string            `env:"ADAPTER_SERVICE_KEYS_DIR"`
	InternalCAFile        string            `env:"ADAPTER_CA_FILE"`
	ClientCAFile          string            `env:"CLIENT_ADAPTER_CA_FILE"`
	InstallationNetworks  []netip.Prefix    `env:"INSTALLATION_NETWORKS"`
	ClientPrivateNetworks []netip.Prefix    `env:"CLIENT_ADAPTER_PRIVATE_NETWORKS"`
	// ServiceKeys are read from ServiceKeysDir by provider.
	ServiceKeys map[string]Secret `env:"-"`
}

func (a Adapters) Secrets() []string {
	out := make([]string, 0, len(a.ServiceKeys))
	for _, k := range a.ServiceKeys {
		out = append(out, k.Value())
	}
	slices.Sort(out)
	return out
}

func LoadAdapters(environ map[string]string) (Adapters, error) {
	var a Adapters
	errs := []error{env.ParseWithOptions(&a, env.Options{Environment: environ})}
	if len(a.ClientPrivateNetworks) > 0 && len(a.InstallationNetworks) == 0 {
		errs = append(errs, errors.New("INSTALLATION_NETWORKS is required when CLIENT_ADAPTER_PRIVATE_NETWORKS is set"))
	}
	if len(a.Builtin) > 0 {
		if a.InternalCAFile == "" {
			errs = append(errs, errors.New("ADAPTER_CA_FILE is required with BUILTIN_ADAPTERS"))
		}
		if a.ServiceKeysDir == "" {
			errs = append(errs, errors.New("ADAPTER_SERVICE_KEYS_DIR is required with BUILTIN_ADAPTERS"))
		}
	}
	a.ServiceKeys = make(map[string]Secret, len(a.Builtin))
	for provider, raw := range a.Builtin {
		if !_providerName.MatchString(provider) {
			errs = append(errs, fmt.Errorf("BUILTIN_ADAPTERS: %q is not a provider name", provider))
			continue
		}
		if u, err := url.Parse(raw); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" {
			errs = append(errs, fmt.Errorf("BUILTIN_ADAPTERS: %s must be an https address without credentials", provider))
		}
		if a.ServiceKeysDir == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(a.ServiceKeysDir, provider))
		var key Secret
		if err == nil {
			err = key.UnmarshalText(b)
		}
		if err != nil || key == "" {
			errs = append(errs, fmt.Errorf("ADAPTER_SERVICE_KEYS_DIR: no service key for %s", provider))
			continue
		}
		a.ServiceKeys[provider] = key
	}
	if err := errors.Join(errs...); err != nil {
		return Adapters{}, fmt.Errorf("invalid adapters: %w", err)
	}
	return a, nil
}
