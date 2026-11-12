package entity

import (
	"errors"
	"time"
)

// Limits of the login and the session (TZ 7.2, 13.1, 13.2).
const (
	LoginAttemptLifetime = 10 * time.Minute
	SessionIdleTimeout   = time.Hour
	SessionLifetime      = 12 * time.Hour
	SessionTouchInterval = time.Minute
	SessionsPerUser      = 10
)

var ErrAuthRequired = errors.New("auth: session is missing, ended or expired")

type SessionEndReason string

const (
	SessionLogout   SessionEndReason = "logout"
	SessionAdmin    SessionEndReason = "admin"
	SessionBlocked  SessionEndReason = "blocked"
	SessionLimit    SessionEndReason = "limit"
	SessionReplaced SessionEndReason = "replaced"
)

// LoginFailure is the class of a failed login that the login page shows (contract completeLogin).
type LoginFailure string

const (
	LoginProviderUnavailable       LoginFailure = "provider_unavailable"
	LoginProviderRejected          LoginFailure = "provider_rejected"
	LoginInvalidResponse           LoginFailure = "invalid_response"
	LoginOrganizationNotConfigured LoginFailure = "organization_not_configured"
	LoginUserBlocked               LoginFailure = "user_blocked"
)

// LoginError is a failed login step; Err keeps the cause for the log, the user sees only Class.
type LoginError struct {
	Class  LoginFailure
	Issuer string
	Err    error
}

func (e *LoginError) Error() string {
	if e.Err == nil {
		return "login: " + string(e.Class)
	}
	return "login: " + string(e.Class) + ": " + e.Err.Error()
}

func (e *LoginError) Unwrap() error { return e.Err }

// ProviderConfig is the OIDC client of one organization with its decrypted secret.
type ProviderConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret Secret
	RedirectURL  string
}

// Identity is the user confirmed by a verified ID token; provider tokens are not kept.
type Identity struct {
	Issuer      string
	Subject     string
	DisplayName string
}

type LoginAttempt struct {
	CookieHash     []byte
	OrganizationID ID
	State          string
	Nonce          string
	PKCEVerifier   string
	Issuer         string
	ReturnPath     string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type LoginStart struct {
	RedirectURL string
	Cookie      string
}

// LoginCallback is the provider response together with the cookies the browser brought back.
type LoginCallback struct {
	Slug          string
	LoginCookie   string
	SessionCookie string
	Code          string
	State         string
	Issuer        string
	Error         string
}

type LoginResult struct {
	SessionToken string
	ReturnPath   string
	ExpiresAt    time.Time
}

type NewSession struct {
	OrganizationID ID
	UserID         ID
	TokenHash      []byte
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type Session struct {
	ID               ID
	OrganizationID   ID
	OrganizationSlug string
	UserID           ID
	CreatedAt        time.Time
	LastActivityAt   time.Time
	ExpiresAt        time.Time
	Ended            bool
}

// Subject is who sent the request: the session keeps only the user and the organization of the login.
type Subject struct {
	OrganizationID   ID
	OrganizationSlug string
	UserID           ID
	SessionID        ID
	ExpiresAt        time.Time
	IdleExpiresAt    time.Time
}
