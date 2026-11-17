package entity

import "time"

type SourceConnection struct {
	ID         ID
	Provider   Provider
	Name       string
	Status     ConnectionStatus
	AccessMode AccessMode
	Areas      []ID
}

type SourceLink struct {
	ConnectionID ID
	Provider     Provider
	DisplayName  string
	State        string
	LinkedAt     *time.Time
	Version      int64
}

type SourceLinkAttempt struct {
	StateHash    []byte
	UserID       ID
	SessionID    ID
	ConnectionID ID
	Version      int64
	ReturnPath   string
	RedirectURI  string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type SourceLinkCallback struct {
	State string
	Code  Secret
	Error string
}

type SourceAuthorization struct {
	UserID      ID
	State       string
	RedirectURI string
}
