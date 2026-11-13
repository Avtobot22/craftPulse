// Package atlassiancheck implements the isolated OAuth acceptance stand, not the product SourceLinks API.
package atlassiancheck

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

var _accountID = regexp.MustCompile(`^[a-zA-Z0-9:-]{1,128}$`)

type Config struct {
	Listen        string            `json:"listen"`
	Callback      string            `json:"callback"`
	CloudID       string            `json:"cloud_id"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"client_secret"`
	ControlSecret string            `json:"control_secret"`
	MasterKey     string            `json:"master_key"`
	Store         string            `json:"store"`
	Accounts      map[string]string `json:"accounts"`
}

func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read stand configuration: %w", err)
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, errors.New("invalid stand configuration JSON")
	}
	return c, c.validate()
}

func (c Config) validate() error {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil || host != "127.0.0.1" {
		return errors.New("listen must use 127.0.0.1 and a port")
	}
	u, err := url.Parse(c.Callback)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.Path != "/source-links/callback" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("callback must be an HTTPS /source-links/callback URL")
	}
	key, err := base64.StdEncoding.DecodeString(c.MasterKey)
	if err != nil || len(key) != 32 || len(c.ControlSecret) < 32 ||
		c.ClientID == "" || c.ClientSecret == "" || !filepath.IsAbs(c.Store) {
		return errors.New("missing credentials, 32-byte key or absolute store path")
	}
	if len(c.Accounts) != 5 || c.Accounts["owner"] == "" || c.Accounts["a_allowed"] == "" ||
		c.Accounts["a_denied"] == "" || c.Accounts["b"] == "" || c.Accounts["collector"] == "" {
		return errors.New("exactly five stand account roles are required")
	}
	seen := make(map[string]bool)
	for _, id := range c.Accounts {
		if !_accountID.MatchString(id) || id == "unknown" || seen[id] {
			return errors.New("account IDs must be valid and distinct")
		}
		seen[id] = true
	}
	if !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(c.CloudID) {
		return errors.New("invalid cloud ID")
	}
	return nil
}
