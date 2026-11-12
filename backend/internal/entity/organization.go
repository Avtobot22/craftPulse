package entity

import "errors"

type OrgStatus string

const (
	OrgStatusSetup    OrgStatus = "setup"
	OrgStatusActive   OrgStatus = "active"
	OrgStatusBlocked  OrgStatus = "blocked"
	OrgStatusDeleting OrgStatus = "deleting"
)

type OIDCProvider struct {
	Issuer       string
	ClientID     string
	ClientSecret Secret
}

// NewOrganization is an organization declared by the installation parameters (FR-064).
type NewOrganization struct {
	ID                ID
	Slug              string
	Name              string
	Provider          OIDCProvider
	FirstAdminSubject string
}

type ProviderRegistration struct {
	OrganizationID  ID
	Issuer          string
	ClientID        string
	ClientSecretEnc []byte
	KeyVersion      uint32
}

var (
	ErrOrganizationNotConfigured = errors.New("org: organization is not configured and the account is not its first administrator")
	ErrOrganizationUnavailable   = errors.New("org: organization is blocked or being deleted")
	ErrUserBlocked               = errors.New("org: user is blocked")
)

type Organization struct {
	ID                ID
	Name              string
	Status            OrgStatus
	FirstAdminIssuer  string
	FirstAdminSubject string
}

type UserStatus string

const (
	UserActive  UserStatus = "active"
	UserBlocked UserStatus = "blocked"
)

type User struct {
	ID          ID
	Issuer      string
	Subject     string
	PersonID    ID
	DisplayName string
	Status      UserStatus
}

// SignIn is the user of a confirmed login; FirstAdministrator marks the login that assigned the role (FR-064).
type SignIn struct {
	User               User
	FirstAdministrator bool
}
