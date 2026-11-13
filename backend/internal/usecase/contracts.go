package usecase

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type Installation interface {
	Apply(ctx context.Context, params entity.InstallParams) ([]entity.AppliedOrganization, error)
	Check(ctx context.Context, params entity.InstallParams) (entity.Protocol, error)
}

// Logins is the auth module for the browser: login through the provider, the session and logout.
type Logins interface {
	StartLogin(ctx context.Context, slug, returnTo string) (entity.LoginStart, error)
	CompleteLogin(ctx context.Context, cb entity.LoginCallback) (entity.LoginResult, error)
	Authenticate(ctx context.Context, token string) (entity.Subject, error)
	Logout(ctx context.Context, s entity.Subject) error
}

// Access decides whether the subject may run the action of an operation (TZ 13.4).
type Access interface {
	Authorize(ctx context.Context, s entity.Subject, r entity.AccessRequest) (entity.Scope, error)
	CurrentUser(ctx context.Context, s entity.Subject) (entity.CurrentUser, error)
}

// Structure is the org module for the confirmed structure of the organization (UC-02, FR-012).
type Structure interface {
	Structure(ctx context.Context, s entity.Subject) (entity.Structure, error)
	ReplaceStructure(ctx context.Context, s entity.Subject, baseVersion int64, in entity.StructureInput) (entity.Structure, error)
	ListAreas(ctx context.Context, s entity.Subject, scope entity.Scope, kind entity.AreaKind, f entity.AreaFilter,
		p entity.PageRequest) (entity.Page, error)
}

// Users is the org module for the administration of users (TZ 7.2, 7.7).
type Users interface {
	ListUsers(ctx context.Context, s entity.Subject, req entity.UserListRequest) (entity.UserPage, error)
	UserRoles(ctx context.Context, s entity.Subject, userID entity.ID) (entity.RoleSet, error)
	ReplaceUserRoles(ctx context.Context, s entity.Subject, userID entity.ID, baseVersion int64, roles []entity.RoleAssignment) (entity.RoleSet, error)
	BlockUser(ctx context.Context, s entity.Subject, userID entity.ID) error
	UnblockUser(ctx context.Context, s entity.Subject, userID entity.ID) error
	EndUserSessions(ctx context.Context, s entity.Subject, userID entity.ID) error
}

// AuditLog reads the audit journal of the organization (TZ 13.8).
type AuditLog interface {
	ListEvents(ctx context.Context, s entity.Subject, q entity.AuditQuery) (entity.AuditPage, error)
}

// Protocols reads the installation check protocol (FR-011).
type Protocols interface {
	LatestProtocol(ctx context.Context) (entity.Protocol, error)
}

type UploadTokens interface {
	CreateUploadToken(context.Context, entity.Subject, entity.ID) (entity.UploadToken, entity.Secret, error)
	ListUploadTokens(context.Context, entity.Subject, entity.ID, int, *entity.AuditKey) (entity.UploadTokenPage, error)
	RevokeUploadToken(context.Context, entity.Subject, entity.ID) error
	AuthenticateUpload(context.Context, string) (entity.Subject, error)
	RejectExposedUpload(context.Context, string) error
}

// Connections is the connect module for administrators (UC-04, UI-09, UI-10, UI-17).
type Connections interface {
	ListAdapters() []entity.AdapterInfo
	List(ctx context.Context, s entity.Subject, f entity.ConnectionFilter) (entity.ConnectionPage, error)
	Create(ctx context.Context, s entity.Subject, in entity.ConnectionInput) (entity.ConnectionState, error)
	State(ctx context.Context, orgID, connectionID entity.ID) (entity.ConnectionState, error)
	Update(ctx context.Context, s entity.Subject, connectionID entity.ID, baseVersion int, p entity.ConnectionPatch) (entity.ConnectionState, error)
	Delete(ctx context.Context, s entity.Subject, connectionID entity.ID) error
	StartProbe(ctx context.Context, s entity.Subject, connectionID entity.ID) (entity.Probe, error)
	GetProbe(ctx context.Context, s entity.Subject, connectionID, probeID entity.ID) (entity.Probe, error)
	Activate(ctx context.Context, s entity.Subject, connectionID entity.ID, baseVersion int, probeID entity.ID) (entity.ConnectionState, error)
}

// ProbeRunner runs the job of a probe; *entity.RetryLaterError asks to run it again later.
type ProbeRunner interface {
	RunProbe(ctx context.Context, orgID, connectionID, probeID entity.ID) error
}

// PackageProcessor runs the job of a quarantined package.
type PackageProcessor interface {
	ProcessPackage(ctx context.Context, orgID, receiptID entity.ID) error
}

// Intake is the ingest module for people: the universal import, receipts and conflicts (UC-06, UI-12, UI-13).
type Intake interface {
	Import(ctx context.Context, s entity.Subject, f entity.ImportFile) (entity.Receipt, error)
	Receipt(ctx context.Context, s entity.Subject, id entity.ID) (entity.Receipt, error)
	ListReceipts(ctx context.Context, s entity.Subject, f entity.ReceiptFilter) (entity.ReceiptPage, error)
	ListRecords(ctx context.Context, s entity.Subject, receiptID entity.ID, f entity.RecordFilter) (entity.RecordPage, error)
	ListConflicts(ctx context.Context, s entity.Subject, f entity.ConflictFilter) (entity.ConflictPage, error)
	ResolveConflict(ctx context.Context, s entity.Subject, id entity.ID, resolution string) (entity.IngestConflict, error)
}

// Collector runs the jobs of collection; complete ends the job in the transaction of its result.
type Collector interface {
	RunPage(ctx context.Context, job entity.SyncPageJob, holder string, complete func(context.Context) error) error
	RunHistory(ctx context.Context, job entity.SyncHistoryJob, holder string, complete func(context.Context) error) error
}

// Normalizer runs the job normalize.batch; complete ends the job in the transaction of its part.
type Normalizer interface {
	RunBatch(ctx context.Context, job entity.NormalizeBatchJob, complete func(context.Context) error) error
}
