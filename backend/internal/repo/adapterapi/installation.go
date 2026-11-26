package adapterapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// CheckBuiltins uses the same TLS clients as normal calls to check the installed release and stores.
func (c *Caller) CheckBuiltins(ctx context.Context, releaseSHA string) (string, error) {
	var details []string
	for _, provider := range slices.Sorted(maps.Keys(c.builtin)) {
		version, err := c.builtin[provider].checkInstallation(ctx, releaseSHA)
		if err != nil {
			return "", fmt.Errorf("adapter %s: %w", provider, err)
		}
		details = append(details, fmt.Sprintf("%s contract %s", provider, version))
	}
	return strings.Join(details, ", "), nil
}

func (b builtinAdapter) checkInstallation(parent context.Context, releaseSHA string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var version struct {
		CommitSHA       string `json:"commit_sha"`
		ContractVersion string `json:"contract_version"`
	}
	err := b.checkEndpoint(ctx, "/version", func(body io.Reader) error {
		return json.NewDecoder(io.LimitReader(body, 4096)).Decode(&version)
	})
	if err != nil {
		return "", err
	}
	if version.CommitSHA != releaseSHA || version.ContractVersion == "" {
		return "", errors.New("version does not identify the release and adapter contract")
	}
	return version.ContractVersion, b.checkEndpoint(ctx, "/readyz", nil)
}

func (b builtinAdapter) checkEndpoint(ctx context.Context, path string, read func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.url+path, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := b.http.Do(req)
	if err != nil {
		failure := transportError(err)
		if failure.Code == entity.AdapterCertificateUntrusted {
			return &entity.CheckFailure{Reason: entity.ReasonCertificate, Detail: "adapter TLS certificate is not trusted"}
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", path, resp.StatusCode)
	}
	if read == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil
	}
	return read(resp.Body)
}
