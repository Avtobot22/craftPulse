package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/caarlos0/env/v11"
)

var _slug = regexp.MustCompile(`^[a-z0-9-]{2,40}$`)

// Install holds the installation parameters of `api install` (UC-01): the site, the organizations of
// the stand with their OIDC provider and first administrator, and the network scope of the client runner.
type Install struct {
	SiteURL       string            `env:"SITE_URL,required,notEmpty"`
	Slugs         []string          `env:"INSTALL_ORGANIZATIONS,required,notEmpty"`
	RunnerNetwork []string          `env:"RUNNER_NETWORK"`
	Images        map[string]string `env:"RELEASE_IMAGES" envKeyValSeparator:"="`
	Organizations []InstallOrganization
}

// InstallOrganization is read from variables ORG_<SLUG>_*, where SLUG is the slug in upper case with
// dashes replaced by underscores. The first administrator belongs to the organization's provider.
type InstallOrganization struct {
	Slug         string
	Name         string `env:"NAME,required,notEmpty"`
	Issuer       string `env:"OIDC_ISSUER,required,notEmpty"`
	ClientID     string `env:"OIDC_CLIENT_ID,required,notEmpty"`
	ClientSecret Secret `env:"OIDC_CLIENT_SECRET_FILE,file,required,notEmpty"`
	AdminSubject string `env:"ADMIN_SUBJECT,required,notEmpty"`
}

func (i Install) Secrets() []string {
	s := make([]string, 0, len(i.Organizations))
	for _, o := range i.Organizations {
		s = append(s, o.ClientSecret.Value())
	}
	return s
}

// LoadInstall reports every missing or invalid parameter at once (UC-01 A1).
func LoadInstall(environ map[string]string) (Install, error) {
	var in Install
	errs := []error{env.ParseWithOptions(&in, env.Options{Environment: environ})}
	if in.SiteURL != "" {
		errs = append(errs, validateHTTPURL("SITE_URL", in.SiteURL))
	}
	for _, p := range in.RunnerNetwork {
		if _, err := netip.ParsePrefix(p); err != nil {
			errs = append(errs, fmt.Errorf("RUNNER_NETWORK: %q is not a CIDR prefix", p))
		}
	}
	seen := make(map[string]bool, len(in.Slugs))
	for _, slug := range in.Slugs {
		if !_slug.MatchString(slug) {
			errs = append(errs, fmt.Errorf("INSTALL_ORGANIZATIONS: slug %q must match %s", slug, _slug))
			continue
		}
		if seen[slug] {
			errs = append(errs, fmt.Errorf("INSTALL_ORGANIZATIONS: slug %q is repeated", slug))
			continue
		}
		seen[slug] = true
		o, err := loadOrganization(environ, slug)
		errs = append(errs, err)
		in.Organizations = append(in.Organizations, o)
	}
	if err := errors.Join(errs...); err != nil {
		return Install{}, fmt.Errorf("invalid installation parameters: %w", err)
	}
	return in, nil
}

func loadOrganization(environ map[string]string, slug string) (InstallOrganization, error) {
	prefix := "ORG_" + strings.ToUpper(strings.ReplaceAll(slug, "-", "_")) + "_"
	o := InstallOrganization{Slug: slug}
	errs := []error{env.ParseWithOptions(&o, env.Options{Environment: environ, Prefix: prefix})}
	if o.Name != "" && utf8.RuneCountInString(o.Name) > 200 {
		errs = append(errs, fmt.Errorf("%sNAME is longer than 200 characters", prefix))
	}
	if o.Issuer != "" {
		if u, err := url.Parse(o.Issuer); err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			errs = append(errs, fmt.Errorf("%sOIDC_ISSUER must be an https URL without query and fragment", prefix))
		}
		if len(o.Issuer) > 512 {
			errs = append(errs, fmt.Errorf("%sOIDC_ISSUER is longer than 512 bytes", prefix))
		}
	}
	if len(o.AdminSubject) > 255 {
		errs = append(errs, fmt.Errorf("%sADMIN_SUBJECT is longer than 255 bytes", prefix))
	}
	if len(o.ClientID) > 255 {
		errs = append(errs, fmt.Errorf("%sOIDC_CLIENT_ID is longer than 255 bytes", prefix))
	}
	return o, errors.Join(errs...)
}
