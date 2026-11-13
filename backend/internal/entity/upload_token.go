package entity

import (
	"errors"
	"time"
)

var ErrTokenSlotsFull = errors.New("auth: both upload token slots are occupied")

type UploadToken struct {
	ID             ID
	OrganizationID ID
	ProjectID      ID
	Slot           int
	CreatedAt      time.Time
	RevokedAt      *time.Time
}

type UploadTokenPage struct {
	Items []UploadToken
	Next  *AuditKey
}
