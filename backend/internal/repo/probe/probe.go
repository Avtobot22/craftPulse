package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _discoveryLimit = 1 << 20

type Probe struct {
	client *http.Client
}

// New takes a client with its own timeout: a probe must finish even when the target never answers.
func New(client *http.Client) *Probe {
	return &Probe{client: client}
}

func (p *Probe) ProbeSite(ctx context.Context, rawURL string) error {
	return p.get(ctx, rawURL, func(resp *http.Response) error {
		if resp.StatusCode >= http.StatusBadRequest {
			return &entity.CheckFailure{Reason: entity.ReasonUnavailable, Detail: fmt.Sprintf("%s answered HTTP %d", host(rawURL), resp.StatusCode)}
		}
		return nil
	})
}

// ProbeIssuer reads the OpenID Connect discovery document; a provider that answers but does not
// describe this issuer is a registration problem rather than a network one.
func (p *Probe) ProbeIssuer(ctx context.Context, issuer string) error {
	return p.get(ctx, strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration", func(resp *http.Response) error {
		return checkDiscovery(resp, issuer)
	})
}

func checkDiscovery(resp *http.Response, issuer string) error {
	if resp.StatusCode != http.StatusOK {
		return &entity.CheckFailure{
			Reason: entity.ReasonRegistration,
			Detail: fmt.Sprintf("%s discovery answered HTTP %d", host(issuer), resp.StatusCode),
		}
	}
	var doc struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		JWKSURI               string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, _discoveryLimit)).Decode(&doc); err != nil {
		return &entity.CheckFailure{Reason: entity.ReasonRegistration, Detail: host(issuer) + " discovery is not valid JSON"}
	}
	switch {
	case doc.Issuer != issuer:
		return &entity.CheckFailure{
			Reason: entity.ReasonRegistration,
			Detail: fmt.Sprintf("discovery names issuer %q instead of %q", doc.Issuer, issuer),
		}
	case doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.JWKSURI == "":
		return &entity.CheckFailure{Reason: entity.ReasonRegistration, Detail: host(issuer) + " discovery lacks authorization, token or JWKS endpoint"}
	}
	return nil
}

// get closes the body after fn, so no response leaks whatever fn returns.
func (p *Probe) get(ctx context.Context, rawURL string, fn func(*http.Response) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return &entity.CheckFailure{Reason: entity.ReasonAddress, Detail: "address is not a valid URL"}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return classify(host(rawURL), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, _discoveryLimit))
		_ = resp.Body.Close()
	}()
	return fn(resp)
}

func classify(target string, err error) *entity.CheckFailure {
	var (
		dnsErr      *net.DNSError
		verifyErr   *tls.CertificateVerificationError
		unknownAuth x509.UnknownAuthorityError
		hostnameErr x509.HostnameError
		invalidErr  x509.CertificateInvalidError
		netErr      net.Error
	)
	switch {
	case errors.As(err, &verifyErr), errors.As(err, &unknownAuth), errors.As(err, &hostnameErr), errors.As(err, &invalidErr):
		return &entity.CheckFailure{Reason: entity.ReasonCertificate, Detail: target + ": TLS certificate is not trusted"}
	case errors.As(err, &dnsErr):
		return &entity.CheckFailure{Reason: entity.ReasonAddress, Detail: target + ": host name does not resolve"}
	case errors.Is(err, syscall.ECONNREFUSED):
		return &entity.CheckFailure{Reason: entity.ReasonAddress, Detail: target + ": connection refused"}
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH),
		errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return &entity.CheckFailure{Reason: entity.ReasonNetworkScope, Detail: target + ": no answer, check the network scope"}
	default:
		return &entity.CheckFailure{Reason: entity.ReasonAddress, Detail: target + ": " + transportMessage(err)}
	}
}

// transportMessage drops the URL that url.Error adds, so query parameters never reach the protocol.
func transportMessage(err error) string {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err.Error()
	}
	return err.Error()
}

func host(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "address"
}
