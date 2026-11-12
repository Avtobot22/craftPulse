package entity

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
