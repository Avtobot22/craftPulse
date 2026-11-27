package github

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

// appCredential is the JSON secret stored for the write_bot purpose.
type appCredential struct {
	ClientID       string `json:"client_id"`
	InstallationID int64  `json:"installation_id"`
	PrivateKey     string `json:"private_key"`
}

type writeBot struct {
	source
	login string
}

var _appSlug = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9-]*$")

func (a *Adapter) writeBot(ctx context.Context, call adaptersdk.Call, collectToken string) (writeBot, error) {
	secret, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeWriteBot)
	if err != nil {
		return writeBot{}, err
	}
	var credential appCredential
	if err := json.Unmarshal([]byte(secret), &credential); err != nil || credential.ClientID == "" || credential.InstallationID <= 0 {
		return writeBot{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitHub write bot credentials require a GitHub App client ID, installation ID and private key")
	}
	jwtToken, err := adaptersdk.SignGitHubAppJWT(credential.ClientID, credential.PrivateKey, time.Now())
	if err != nil {
		return writeBot{}, err
	}
	issuer := source{client: a.client, token: jwtToken}
	var app struct {
		Slug string `json:"slug"`
	}
	if err := issuer.get(ctx, "write_bot_app", "/app", &app); err != nil {
		return writeBot{}, err
	}
	if !_appSlug.MatchString(app.Slug) {
		return writeBot{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub App identity is missing")
	}
	var grant struct {
		Token       string            `json:"token"`
		Permissions map[string]string `json:"permissions"`
	}
	req := map[string]map[string]string{"permissions": {"pull_requests": "write"}}
	path := "/app/installations/" + id(credential.InstallationID) + "/access_tokens"
	if err := issuer.sendJSON(ctx, "write_bot_token", http.MethodPost, path, req, &grant); err != nil {
		if e, ok := errors.AsType[*adaptersdk.Error](err); ok && e.SourceStatusCode() == http.StatusUnprocessableEntity {
			return writeBot{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitHub App cannot grant pull requests write permission")
		}
		return writeBot{}, err
	}
	// GitHub adds metadata read to the requested pull request permission.
	expectedPermissions := map[string]string{"metadata": "read", "pull_requests": "write"}
	if !maps.Equal(grant.Permissions, expectedPermissions) || grant.Token == "" || grant.Token == collectToken ||
		strings.ContainsAny(grant.Token, " \t\r\n") {
		return writeBot{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "GitHub App must grant a separate token limited to pull requests write and metadata read")
	}
	return writeBot{source: source{client: a.client, token: grant.Token}, login: app.Slug + "[bot]"}, nil
}

func (b writeBot) walkRepositories(ctx context.Context, visit func(repository) error) error {
	for page := 1; page != 0; {
		repos, next, err := b.repositoryPage(ctx, page)
		if err != nil {
			return err
		}
		for i := range repos {
			if !repos[i].validIdentity() {
				return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub App repository identity is incomplete")
			}
			if err := visit(repos[i]); err != nil {
				return err
			}
		}
		page = next
	}
	return nil
}

func (b writeBot) repositoryPage(ctx context.Context, page int) ([]repository, int, error) {
	query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}}
	resp, err := b.request(ctx, "write_bot_repositories", "/installation/repositories?"+query.Encode()) //nolint:bodyclose // DecodeJSON owns the body
	if err != nil {
		return nil, 0, err
	}
	links := resp.Header.Values("Link")
	var reply struct {
		Repositories *[]repository `json:"repositories"`
	}
	if err := adaptersdk.DecodeJSON(resp, &reply); err != nil {
		return nil, 0, err
	}
	if reply.Repositories == nil {
		return nil, 0, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitHub App repository list is missing")
	}
	next, err := nextPage(links, page)
	return *reply.Repositories, next, err
}
