package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_operationLogin  = "session.login"
	_operationLogout = "session.logout"
	_tokenBytes      = 32
	_maxReturnPath   = 2048
)

var _tokenLength = base64.RawURLEncoding.EncodedLen(_tokenBytes)

// loginTrace collects what a failed login records in the audit: never tokens, state or nonce (TZ 13.8).
type loginTrace struct {
	orgID   entity.ID
	issuer  string
	subject string
	userID  entity.ID
}

// StartLogin creates the one-time login attempt and returns the provider address (TZ 13.1, steps 1-3).
func (u *UseCase) StartLogin(ctx context.Context, slug, returnTo string) (entity.LoginStart, error) {
	var trace loginTrace
	start, err := u.begin(ctx, slug, returnTo, &trace)
	if err != nil {
		return entity.LoginStart{}, u.loginFailed(ctx, trace, err)
	}
	return start, nil
}

func (u *UseCase) begin(ctx context.Context, slug, returnTo string, trace *loginTrace) (entity.LoginStart, error) {
	orgID, reg, err := u.registration(ctx, slug)
	if err != nil {
		return entity.LoginStart{}, err
	}
	trace.orgID, trace.issuer = orgID, reg.Issuer

	cookie, state, nonce, verifier := newToken(), newToken(), newToken(), newToken()
	redirect, err := u.d.Provider.AuthCodeURL(ctx, u.config(slug, reg, ""), state, nonce, verifier)
	if err != nil {
		return entity.LoginStart{}, err
	}
	now := u.d.Clock.Now()
	err = u.d.Repo.CreateLoginAttempt(ctx, entity.LoginAttempt{
		CookieHash:     hashToken(cookie),
		OrganizationID: orgID,
		State:          state,
		Nonce:          nonce,
		PKCEVerifier:   verifier,
		Issuer:         reg.Issuer,
		ReturnPath:     ReturnPath(returnTo),
		CreatedAt:      now,
		ExpiresAt:      now.Add(entity.LoginAttemptLifetime),
	})
	if err != nil {
		return entity.LoginStart{}, err
	}
	return entity.LoginStart{RedirectURL: redirect, Cookie: cookie}, nil
}

// CompleteLogin checks the provider response and creates the session (TZ 13.1, steps 4-8). Any failure
// leaves no session and returns *entity.LoginError when the cause is a login step.
func (u *UseCase) CompleteLogin(ctx context.Context, cb entity.LoginCallback) (entity.LoginResult, error) {
	var trace loginTrace
	res, err := u.finish(ctx, cb, &trace)
	if err != nil {
		return entity.LoginResult{}, u.loginFailed(ctx, trace, err)
	}
	return res, nil
}

func (u *UseCase) finish(ctx context.Context, cb entity.LoginCallback, trace *loginTrace) (entity.LoginResult, error) {
	orgID, reg, err := u.registration(ctx, cb.Slug)
	if err != nil {
		return entity.LoginResult{}, err
	}
	trace.orgID, trace.issuer = orgID, reg.Issuer

	attempt, err := u.takeAttempt(ctx, orgID, cb)
	if err != nil {
		return entity.LoginResult{}, err
	}
	secret, err := u.d.Keyring.OpenEnvelope(reg.ClientSecretEnc, reg.KeyVersion, ClientSecretAAD(orgID))
	if err != nil {
		return entity.LoginResult{}, fmt.Errorf("auth: open client secret: %w", err)
	}
	id, err := u.d.Provider.Exchange(ctx, u.config(cb.Slug, reg, entity.Secret(secret)), cb.Code, attempt.PKCEVerifier, attempt.Nonce)
	if err != nil {
		return entity.LoginResult{}, err
	}
	trace.subject = id.Subject

	token := newToken()
	now := u.d.Clock.Now()
	res := entity.LoginResult{SessionToken: token, ReturnPath: attempt.ReturnPath, ExpiresAt: now.Add(entity.SessionLifetime)}
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		in, err := u.d.Users.SignIn(ctx, orgID, id, now)
		if err != nil {
			return signInFailure(err)
		}
		trace.userID = in.User.ID
		if err := u.openSession(ctx, orgID, in.User.ID, cb.SessionCookie, hashToken(token), res.ExpiresAt); err != nil {
			return err
		}
		details := map[string]any{"issuer": id.Issuer, "subject": id.Subject}
		if in.FirstAdministrator {
			details["assigned_role"] = string(entity.RoleAdministrator)
		}
		return u.d.Auditor.Record(ctx, entity.AuditEvent{
			OrganizationID: orgID,
			OccurredAt:     now,
			ActorKind:      entity.ActorUser,
			ActorID:        in.User.ID,
			Operation:      _operationLogin,
			ObjectKind:     "user",
			ObjectID:       in.User.ID,
			Outcome:        entity.AuditSucceeded,
			CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
			Details:        details,
		})
	})
	if err != nil {
		return entity.LoginResult{}, err
	}
	return res, nil
}

// takeAttempt removes the attempt before checking it: a replayed or forged response finds nothing next time.
func (u *UseCase) takeAttempt(ctx context.Context, orgID entity.ID, cb entity.LoginCallback) (entity.LoginAttempt, error) {
	if len(cb.LoginCookie) != _tokenLength {
		return entity.LoginAttempt{}, invalidResponse("no login attempt cookie")
	}
	a, found, err := u.d.Repo.TakeLoginAttempt(ctx, hashToken(cb.LoginCookie))
	switch {
	case err != nil:
		return entity.LoginAttempt{}, err
	case !found:
		return entity.LoginAttempt{}, invalidResponse("login attempt is not found or was already used")
	case a.OrganizationID != orgID:
		return entity.LoginAttempt{}, invalidResponse("login attempt belongs to another organization")
	case !u.d.Clock.Now().Before(a.ExpiresAt):
		return entity.LoginAttempt{}, invalidResponse("login attempt has expired")
	case subtle.ConstantTimeCompare([]byte(cb.State), []byte(a.State)) != 1:
		return entity.LoginAttempt{}, invalidResponse("state does not match the login attempt")
	case cb.Issuer != "" && cb.Issuer != a.Issuer:
		return entity.LoginAttempt{}, invalidResponse("iss does not match the provider of the organization")
	case cb.Error != "":
		return entity.LoginAttempt{}, &entity.LoginError{Class: entity.LoginProviderRejected, Err: fmt.Errorf("provider returned error %q", cb.Error)}
	case cb.Code == "":
		return entity.LoginAttempt{}, invalidResponse("provider response has no code")
	}
	return a, nil
}

// openSession ends the session the browser came with and the oldest sessions over the limit (TZ 13.2).
func (u *UseCase) openSession(ctx context.Context, orgID, userID entity.ID, browserCookie string, tokenHash []byte, expiresAt time.Time) error {
	now := u.d.Clock.Now()
	if browserCookie != "" {
		if err := u.d.Repo.EndSessionByToken(ctx, hashToken(browserCookie), now, entity.SessionReplaced); err != nil {
			return err
		}
	}
	live, err := u.d.Repo.LiveSessions(ctx, orgID, userID, now, now.Add(-entity.SessionIdleTimeout))
	if err != nil {
		return err
	}
	if over := len(live) - entity.SessionsPerUser + 1; over > 0 {
		if err := u.d.Repo.EndSessions(ctx, orgID, live[:over], now, entity.SessionLimit); err != nil {
			return err
		}
	}
	return u.d.Repo.CreateSession(ctx, entity.NewSession{
		OrganizationID: orgID,
		UserID:         userID,
		TokenHash:      tokenHash,
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
	})
}

// registration resolves the login route; an unknown organization fails like an unavailable provider.
func (u *UseCase) registration(ctx context.Context, slug string) (entity.ID, entity.ProviderRegistration, error) {
	orgID, found, err := u.d.Repo.OrganizationBySlug(ctx, slug)
	if err != nil {
		return entity.ID{}, entity.ProviderRegistration{}, err
	}
	if !found {
		return entity.ID{}, entity.ProviderRegistration{}, &entity.LoginError{
			Class: entity.LoginProviderUnavailable, Err: errors.New("login route is not found"),
		}
	}
	var reg entity.ProviderRegistration
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		r, ok, err := u.d.Repo.ActiveProviderRegistration(ctx, orgID)
		if err != nil {
			return err
		}
		if !ok {
			return &entity.LoginError{Class: entity.LoginProviderUnavailable, Err: errors.New("organization has no active provider registration")}
		}
		reg = r
		return nil
	})
	return orgID, reg, err
}

func (u *UseCase) config(slug string, reg entity.ProviderRegistration, secret entity.Secret) entity.ProviderConfig {
	return entity.ProviderConfig{
		Issuer:       reg.Issuer,
		ClientID:     reg.ClientID,
		ClientSecret: secret,
		RedirectURL:  u.d.SiteURL + "/o/" + url.PathEscape(slug) + "/callback",
	}
}

// loginFailed logs the cause and records the failed login separately: the login transaction is rolled back.
// The user still gets the failure class when the audit is unavailable.
func (u *UseCase) loginFailed(ctx context.Context, trace loginTrace, err error) error {
	le, ok := errors.AsType[*entity.LoginError](err)
	if !ok {
		return err
	}
	if le.Issuer == "" {
		le.Issuer = trace.issuer
	}
	log := zerolog.Ctx(ctx)
	log.Warn().Err(le.Err).Str("reason", string(le.Class)).Str("issuer", le.Issuer).Msg("login failed")
	if trace.orgID.IsZero() {
		return le
	}
	outcome := entity.AuditFailed
	if le.Class == entity.LoginUserBlocked || le.Class == entity.LoginOrganizationNotConfigured {
		outcome = entity.AuditDenied
	}
	details := map[string]any{"issuer": le.Issuer}
	if trace.subject != "" {
		details["subject"] = trace.subject
	}
	auditErr := u.d.Auditor.RecordSeparately(ctx, entity.AuditEvent{
		OrganizationID: trace.orgID,
		ActorKind:      entity.ActorUser,
		ActorID:        trace.userID,
		Operation:      _operationLogin,
		Outcome:        outcome,
		ReasonCode:     string(le.Class),
		CorrelationID:  entity.ID(logger.CorrelationID(ctx)),
		Details:        details,
	})
	if auditErr != nil {
		log.Error().Err(auditErr).Msg("failed login is not recorded in the audit")
	}
	return le
}

func signInFailure(err error) error {
	switch {
	case errors.Is(err, entity.ErrUserBlocked):
		return &entity.LoginError{Class: entity.LoginUserBlocked, Err: err}
	case errors.Is(err, entity.ErrOrganizationNotConfigured):
		return &entity.LoginError{Class: entity.LoginOrganizationNotConfigured, Err: err}
	case errors.Is(err, entity.ErrOrganizationUnavailable):
		return &entity.LoginError{Class: entity.LoginProviderUnavailable, Err: err}
	}
	return err
}

func invalidResponse(cause string) *entity.LoginError {
	return &entity.LoginError{Class: entity.LoginInvalidResponse, Err: errors.New(cause)}
}

// ReturnPath keeps a relative path of the site; anything else sends the user to the start page (UI-02).
func ReturnPath(raw string) string {
	const home = "/"
	if raw == "" || len(raw) > _maxReturnPath || raw[0] != '/' || strings.HasPrefix(raw, "//") || hasUnsafe(raw) {
		return home
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || hasUnsafe(u.Path) ||
		strings.HasPrefix(u.Path, "//") || strings.HasPrefix(path.Clean(u.Path), "//") {
		return home
	}
	return raw
}

func hasUnsafe(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == '\\' || r == 0x7f })
}

func newToken() string {
	b := make([]byte, _tokenBytes)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
