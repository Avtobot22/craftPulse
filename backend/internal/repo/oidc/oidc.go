package oidc

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _maxSubject = 255

// Client keeps one discovered provider per issuer: discovery and signing keys come only from the issuer.
type Client struct {
	http      *http.Client
	mu        sync.Mutex
	providers map[string]*oidc.Provider
}

// New takes a client with its own timeout: a login must not hang on a provider that never answers.
func New(client *http.Client) *Client {
	return &Client{http: client, providers: make(map[string]*oidc.Provider)}
}

func (c *Client) AuthCodeURL(ctx context.Context, p entity.ProviderConfig, state, nonce, verifier string) (string, error) {
	provider, err := c.provider(ctx, p.Issuer)
	if err != nil {
		return "", err
	}
	return config(provider, p).AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("response_mode", "query"),
	), nil
}

// Exchange trades the code for tokens and returns only the identity of the verified ID token.
func (c *Client) Exchange(ctx context.Context, p entity.ProviderConfig, code, verifier, nonce string) (entity.Identity, error) {
	provider, err := c.provider(ctx, p.Issuer)
	if err != nil {
		return entity.Identity{}, err
	}
	ctx = oidc.ClientContext(ctx, c.http)
	token, err := config(provider, p).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		if _, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
			return entity.Identity{}, failure(entity.LoginProviderRejected, p.Issuer, fmt.Errorf("token endpoint: %w", err))
		}
		return entity.Identity{}, failure(entity.LoginProviderUnavailable, p.Issuer, fmt.Errorf("token endpoint: %w", err))
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		return entity.Identity{}, failure(entity.LoginInvalidResponse, p.Issuer, errors.New("token response has no id_token"))
	}
	idToken, err := provider.VerifierContext(ctx, &oidc.Config{ClientID: p.ClientID}).Verify(ctx, raw)
	if err != nil {
		return entity.Identity{}, failure(entity.LoginInvalidResponse, p.Issuer, fmt.Errorf("verify id_token: %w", err))
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 {
		return entity.Identity{}, failure(entity.LoginInvalidResponse, p.Issuer, errors.New("id_token nonce does not match the login attempt"))
	}
	if idToken.Subject == "" || len(idToken.Subject) > _maxSubject {
		return entity.Identity{}, failure(entity.LoginInvalidResponse, p.Issuer, errors.New("id_token sub is empty or too long"))
	}
	var claims struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return entity.Identity{}, failure(entity.LoginInvalidResponse, p.Issuer, fmt.Errorf("decode id_token claims: %w", err))
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	return entity.Identity{Issuer: idToken.Issuer, Subject: idToken.Subject, DisplayName: name}, nil
}

func (c *Client) provider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	c.mu.Lock()
	p, ok := c.providers[issuer]
	c.mu.Unlock()
	if ok {
		return p, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, c.http), issuer)
	if err != nil {
		return nil, failure(entity.LoginProviderUnavailable, issuer, fmt.Errorf("discovery: %w", err))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.providers[issuer]; ok {
		return cached, nil
	}
	c.providers[issuer] = p
	return p, nil
}

func config(provider *oidc.Provider, p entity.ProviderConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret.Value(),
		Endpoint:     provider.Endpoint(),
		RedirectURL:  p.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile"},
	}
}

func failure(class entity.LoginFailure, issuer string, err error) *entity.LoginError {
	return &entity.LoginError{Class: class, Issuer: issuer, Err: err}
}
