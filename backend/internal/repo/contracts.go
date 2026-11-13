package repo

import (
	"context"
	"io"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type JobQueue interface {
	Enqueue(ctx context.Context, job entity.Job, queue string) error
	// EnqueueAt puts a job that is not taken before at.
	EnqueueAt(ctx context.Context, job entity.Job, queue string, at time.Time) error
}

// Transactor opens transactions; repositories take the transaction from ctx (TZ 10.3, rule 2).
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
	InOrgTx(ctx context.Context, orgID entity.ID, fn func(ctx context.Context) error) error
}

type TenantRepo interface {
	CreateTenant(ctx context.Context, orgID entity.ID, at time.Time) error
	NextSeq(ctx context.Context, orgID entity.ID) (int64, error)
}

// OrgRepo reads and writes tables under row-level security: every call runs in an organization transaction.
type OrgRepo interface {
	CreateOrganization(ctx context.Context, org entity.NewOrganization, at time.Time) error
	Organization(ctx context.Context, orgID entity.ID) (entity.Organization, error)
	LockOrganization(ctx context.Context, orgID entity.ID) (entity.Organization, error)
	ActivateOrganization(ctx context.Context, orgID, userID entity.ID) error
	LockUserByIdentity(ctx context.Context, orgID entity.ID, issuer, subject string) (entity.User, bool, error)
	CreateUser(ctx context.Context, orgID entity.ID, id entity.Identity, at time.Time) error
	SetUserDisplayName(ctx context.Context, orgID, userID entity.ID, name string) error
	User(ctx context.Context, orgID, userID entity.ID) (entity.User, error)
	AssignAdministrator(ctx context.Context, orgID, userID entity.ID, at time.Time) error
	ActiveRoles(ctx context.Context, orgID, userID entity.ID) ([]entity.RoleAssignment, error)
	MemberTeamIDs(ctx context.Context, orgID, personID entity.ID, at time.Time) ([]entity.ID, error)
	TeamProjectIDs(ctx context.Context, orgID entity.ID, teamIDs []entity.ID, at time.Time) ([]entity.ID, error)
	VisibleAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, whole bool, ids []entity.ID) ([]entity.Area, error)
	StructureRepo
	UserRepo
	SourceAccountRepo
}

// SourceAccountRepo keeps the accounts of source instances (TZ 12.5.1); a name or an email joins nothing.
type SourceAccountRepo interface {
	EnsureSourceAccounts(ctx context.Context, orgID, instanceID entity.ID, accounts []entity.SourceAccount, at time.Time) (map[string]entity.ID, error)
}

// UserRepo is the administration of users: roles and blocking (TZ 7.2, 7.7).
type UserRepo interface {
	LockUser(ctx context.Context, orgID, userID entity.ID) (entity.User, error)
	SetUserStatus(ctx context.Context, orgID, userID entity.ID, status entity.UserStatus, blockedAt time.Time) error
	RoleSetVersion(ctx context.Context, orgID, userID entity.ID) (int64, error)
	RevokeRole(ctx context.Context, orgID, assignmentID, by entity.ID, at time.Time) error
	AssignRole(ctx context.Context, orgID, userID entity.ID, ra entity.RoleAssignment, by entity.ID, at time.Time) error
	ExistingAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, ids []entity.ID) ([]entity.ID, error)
	PageUsers(ctx context.Context, orgID entity.ID, req entity.UserListRequest) ([]entity.User, error)
}

// StructureRepo is the confirmed structure of the organization (TZ 12.5.1).
type StructureRepo interface {
	LockStructureVersion(ctx context.Context, orgID entity.ID) (int64, error)
	StructureVersion(ctx context.Context, orgID entity.ID) (int64, error)
	SetStructureVersion(ctx context.Context, orgID entity.ID, version int64) error
	ExistingPersons(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ID, error)
	AllAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind) ([]entity.Area, error)
	CreateArea(ctx context.Context, orgID entity.ID, kind entity.AreaKind, a entity.Area, at time.Time) error
	UpdateArea(ctx context.Context, orgID entity.ID, kind entity.AreaKind, a entity.Area) error
	CurrentLinks(ctx context.Context, orgID entity.ID, kind entity.LinkKind, at time.Time) ([]entity.Link, error)
	SupersedeLink(ctx context.Context, orgID entity.ID, kind entity.LinkKind, rowID entity.ID, seq int64, at time.Time) error
	InsertLink(ctx context.Context, orgID entity.ID, kind entity.LinkKind, l entity.Link, seq int64) error
	PageAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, scope entity.Scope, f entity.AreaFilter,
		p entity.PageRequest, at time.Time) ([]entity.Area, error)
}

// AuthRepo serves login routes, login attempts and sessions outside row-level security and provider
// registrations under it.
type AuthRepo interface {
	OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error)
	CreateLoginRoute(ctx context.Context, slug string, orgID entity.ID) error
	CreateProviderRegistration(ctx context.Context, reg entity.ProviderRegistration, at time.Time) error
	ActiveProviderRegistration(ctx context.Context, orgID entity.ID) (entity.ProviderRegistration, bool, error)
	CreateLoginAttempt(ctx context.Context, a entity.LoginAttempt) error
	TakeLoginAttempt(ctx context.Context, cookieHash []byte) (entity.LoginAttempt, bool, error)
	CreateSession(ctx context.Context, s entity.NewSession) error
	SessionByToken(ctx context.Context, tokenHash []byte) (entity.Session, bool, error)
	TouchSession(ctx context.Context, orgID, sessionID entity.ID, at time.Time) error
	EndSessionByToken(ctx context.Context, tokenHash []byte, at time.Time, reason entity.SessionEndReason) error
	EndSession(ctx context.Context, orgID, sessionID entity.ID, at time.Time, reason entity.SessionEndReason) (bool, error)
	LiveSessions(ctx context.Context, orgID, userID entity.ID, at, idleSince time.Time) ([]entity.ID, error)
	EndSessions(ctx context.Context, orgID entity.ID, ids []entity.ID, at time.Time, reason entity.SessionEndReason) error
	EndUserSessions(ctx context.Context, orgID, userID entity.ID, at time.Time, reason entity.SessionEndReason) (int64, error)
	RevokeUserAPIKeys(ctx context.Context, orgID, userID entity.ID, at time.Time) (int64, error)
}

// IdentityProvider is the OpenID Connect client; errors are *entity.LoginError with the failure class.
type IdentityProvider interface {
	AuthCodeURL(ctx context.Context, p entity.ProviderConfig, state, nonce, verifier string) (string, error)
	Exchange(ctx context.Context, p entity.ProviderConfig, code, verifier, nonce string) (entity.Identity, error)
}

type AuditRepo interface {
	InsertEvent(ctx context.Context, e entity.AuditEvent) error
	PageEvents(ctx context.Context, orgID entity.ID, q entity.AuditQuery) ([]entity.AuditEvent, error)
}

type InstallRepo interface {
	CreateProtocol(ctx context.Context, startedAt time.Time, releaseSHA string, manifest []byte) (entity.ID, error)
	AddCheckResult(ctx context.Context, protocolID entity.ID, r entity.CheckResult) error
	FinishProtocol(ctx context.Context, protocolID entity.ID, finishedAt time.Time, outcome entity.ProtocolOutcome) error
	DatabaseFacts(ctx context.Context) (entity.DatabaseFacts, error)
	LatestProtocol(ctx context.Context) (entity.Protocol, bool, error)
}

// SiteProbe and IdentityProviderProbe return *entity.CheckFailure when the component is not usable.
type SiteProbe interface {
	ProbeSite(ctx context.Context, url string) error
}

type IdentityProviderProbe interface {
	ProbeIssuer(ctx context.Context, issuer string) error
}

type ObjectStoreProbe interface {
	Check(ctx context.Context) error
}

type UploadTokenRepo interface {
	CreateUploadToken(context.Context, entity.ID, entity.ID, entity.ID, []byte, time.Time) (entity.UploadToken, error)
	UploadToken(context.Context, entity.ID, entity.ID) (entity.UploadToken, error)
	UploadTokenByHash(context.Context, []byte) (entity.UploadToken, error)
	RevokeUploadToken(context.Context, entity.ID, entity.ID, time.Time) (bool, error)
	PageUploadTokens(context.Context, entity.ID, entity.ID, int, *entity.AuditKey) ([]entity.UploadToken, error)
}

// ConnectRepo is the connect schema under row-level security: every call runs in an organization transaction.
type ConnectRepo interface {
	EnsureInstance(ctx context.Context, orgID entity.ID, provider entity.Provider, key string) (entity.SourceInstance, error)
	Instance(ctx context.Context, orgID, id entity.ID) (entity.SourceInstance, error)
	SetInstanceBaseURL(ctx context.Context, orgID, id entity.ID, baseURL string) error
	VerifyInstance(ctx context.Context, in entity.SourceInstance) error
	CreateConnection(ctx context.Context, c entity.Connection) error
	Connection(ctx context.Context, orgID, id entity.ID) (entity.Connection, error)
	LockConnection(ctx context.Context, orgID, id entity.ID) (entity.Connection, error)
	PageConnections(ctx context.Context, orgID entity.ID, f entity.ConnectionFilter) ([]entity.Connection, error)
	SaveConnection(ctx context.Context, c entity.Connection) error
	PauseConnection(ctx context.Context, orgID, id entity.ID, until time.Time) error
	MarkAccessLost(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error)
	AddScope(ctx context.Context, orgID, connectionID, instanceID entity.ID, s entity.NewScope, at time.Time) (entity.ID, error)
	Scopes(ctx context.Context, orgID, connectionID entity.ID) ([]entity.ConnectionScope, error)
	ScopesOf(ctx context.Context, orgID entity.ID, connectionIDs []entity.ID) (map[entity.ID][]entity.ConnectionScope, error)
	RemoveScope(ctx context.Context, orgID, scopeID entity.ID, at time.Time) error
	RemoveScopes(ctx context.Context, orgID, connectionID entity.ID, at time.Time) error
	SetScopeHistory(ctx context.Context, orgID, scopeID entity.ID, from *time.Time) error
	SetScopeArea(ctx context.Context, orgID, scopeID, areaID entity.ID) (bool, error)
	Capabilities(ctx context.Context, orgID, connectionID entity.ID) ([]entity.Capability, error)
	CapabilitiesOf(ctx context.Context, orgID entity.ID, connectionIDs []entity.ID) (map[entity.ID][]entity.Capability, error)
	ReplaceCapabilities(ctx context.Context, orgID, connectionID entity.ID, caps []entity.Capability) error
	RunningProbe(ctx context.Context, orgID, connectionID entity.ID) (entity.Probe, bool, error)
	CreateProbe(ctx context.Context, orgID, connectionID entity.ID, configurationVersion int, at time.Time) (entity.Probe, error)
	Probe(ctx context.Context, orgID, connectionID, probeID entity.ID) (entity.Probe, error)
	LatestProbe(ctx context.Context, orgID, connectionID entity.ID) (entity.Probe, bool, error)
	StartProbeRun(ctx context.Context, orgID, probeID entity.ID) (bool, error)
	FinishProbe(ctx context.Context, orgID entity.ID, p entity.Probe) error
}

// AddressChecker refuses an address an administrator enters before it is saved (TZ 13.7); the same
// rules hold again on every connection. Errors are entity.ErrOutsideScope.
type AddressChecker interface {
	CheckSourceURL(ctx context.Context, raw string) error
	CheckAdapterURL(ctx context.Context, raw string) error
}

// AdapterClient is the transport of adapter.yaml for connect; every error is *entity.AdapterError.
type AdapterClient interface {
	PutCredentials(ctx context.Context, t entity.AdapterTarget, in entity.CredentialsInput) error
	DeleteCredentials(ctx context.Context, t entity.AdapterTarget, f entity.CredentialsFilter) error
	Probe(ctx context.Context, t entity.AdapterTarget, in entity.ProbeInput) (entity.ProbeResult, error)
	// Fetch returns the package stream; closing it ends the call.
	Fetch(ctx context.Context, t entity.AdapterTarget, in entity.FetchInput) (io.ReadCloser, error)
	Manifest(ctx context.Context, t entity.AdapterTarget, in entity.ManifestInput) (entity.ManifestPage, error)
	CheckAccess(ctx context.Context, t entity.AdapterTarget, in entity.SourceAccessInput) (entity.SourceAccessResult, error)
}

// IngestRepo is the journal of intake under row-level security.
type IngestRepo interface {
	SaveReceipt(ctx context.Context, r entity.Receipt) error
	Receipt(ctx context.Context, orgID, id entity.ID) (entity.Receipt, error)
	LockReceipt(ctx context.Context, orgID, id entity.ID) (entity.Receipt, error)
	FirstReceiptOfHash(ctx context.Context, orgID entity.ID, hash []byte) (entity.ID, bool, error)
	LockPackageHash(ctx context.Context, orgID entity.ID, hash []byte) error
	AcceptRecords(ctx context.Context, orgID, receiptID entity.ID, recs []entity.IngestRecord) ([]bool, error)
	AcceptedRecords(ctx context.Context, orgID entity.ID, recs []entity.IngestRecord) ([]entity.IngestRecord, error)
	InsertRecords(ctx context.Context, orgID, receiptID entity.ID, recs []entity.IngestRecord) error
	InsertConflicts(ctx context.Context, orgID, receiptID entity.ID, recs []entity.IngestRecord, at time.Time) error
	Records(ctx context.Context, orgID, receiptID entity.ID) ([]entity.IngestRecord, error)
	PageReceipts(ctx context.Context, orgID entity.ID, f entity.ReceiptFilter) ([]entity.Receipt, error)
	CountRepeats(ctx context.Context, orgID entity.ID, receipts []entity.Receipt) error
	PageRecords(ctx context.Context, orgID, receiptID entity.ID, f entity.RecordFilter) ([]entity.IngestRecord, error)
	Record(ctx context.Context, orgID, receiptID entity.ID, no int) (entity.IngestRecord, error)
	SetRecordOutcome(ctx context.Context, orgID, receiptID entity.ID, rec entity.IngestRecord) error
	PageConflicts(ctx context.Context, orgID entity.ID, f entity.ConflictFilter) ([]entity.IngestConflict, error)
	LockConflict(ctx context.Context, orgID, id entity.ID) (entity.IngestConflict, error)
	ResolveConflict(ctx context.Context, orgID entity.ID, c entity.IngestConflict) error
}

// ObjectStore keeps packages; Get of a missing object is entity.ErrNotFound and Delete of one succeeds.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// SyncRepo is the state of collection and the lease of a connection under row-level security.
type SyncRepo interface {
	State(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.SyncState, bool, error)
	SaveState(ctx context.Context, orgID entity.ID, s entity.SyncState) error
	// AcquireLease takes or renews the lease; false means another holder has it until it runs out.
	AcquireLease(ctx context.Context, orgID, connectionID entity.ID, holder string, until, now time.Time) (bool, error)
	ReleaseLease(ctx context.Context, orgID, connectionID entity.ID, holder string) error
}

// NormalizeRepo is the normalize schema under row-level security: canonical facts in versions of knowledge.
type NormalizeRepo interface {
	// EnsureObjects creates the identities that are new with the knowledge number seq and returns all by key.
	EnsureObjects(ctx context.Context, orgID entity.ID, keys []entity.SourceKey, quality entity.IdentityQuality, seq int64) (map[entity.SourceKey]entity.SourceObject, error)
	EnsureAreas(ctx context.Context, orgID entity.ID, areas map[entity.ID]entity.ObjectKind) error
	PlaceObjects(ctx context.Context, orgID entity.ID, placements []entity.ObjectPlacement) error
	// RecordAreaChanges writes the changes of areas at seq and moves their last change to it.
	RecordAreaChanges(ctx context.Context, orgID entity.ID, seq int64, changes []entity.AreaChange, at time.Time) error
	AreaChanges(ctx context.Context, orgID, areaID entity.ID, afterSeq int64) ([]entity.AreaChange, error)
	AreaOf(ctx context.Context, orgID, instanceID entity.ID, kind entity.ObjectKind, nativeID string) (entity.ID, error)

	CurrentTaskVersions(ctx context.Context, orgID entity.ID, taskIDs []entity.ID) ([]entity.TaskVersion, error)
	TaskRevisions(ctx context.Context, orgID entity.ID, taskIDs []entity.ID) (map[entity.ID][]entity.Revision, error)
	SupersedeTaskVersions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertTaskVersions(ctx context.Context, orgID entity.ID, versions []entity.TaskVersion) error
	TaskVersionsAt(ctx context.Context, orgID, areaID entity.ID, seq int64) ([]entity.TaskVersion, error)
	CurrentTasksByNative(ctx context.Context, orgID, areaID entity.ID, nativeIDs []string) (map[string]entity.TaskVersion, error)

	CurrentTaskEvents(ctx context.Context, orgID entity.ID, keys []entity.EventKey) ([]entity.TaskEvent, error)
	TaskEventRevisions(ctx context.Context, orgID entity.ID, keys []entity.EventKey) (map[entity.EventKey][]entity.Revision, error)
	SupersedeTaskEvents(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertTaskEvents(ctx context.Context, orgID entity.ID, events []entity.TaskEvent) error
	TaskEventsAt(ctx context.Context, orgID, areaID entity.ID, seq int64) ([]entity.TaskEvent, error)

	CurrentDictionaryEntries(ctx context.Context, orgID entity.ID, keys []entity.DictionaryKey) ([]entity.DictionaryEntry, error)
	DictionaryRevisions(ctx context.Context, orgID entity.ID, keys []entity.DictionaryKey) (map[entity.DictionaryKey][]entity.Revision, error)
	SupersedeDictionaryEntries(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertDictionaryEntries(ctx context.Context, orgID entity.ID, entries []entity.DictionaryEntry) error
	Dictionary(ctx context.Context, orgID, areaID entity.ID, kind entity.DictionaryKind) ([]entity.DictionaryEntry, error)
}
