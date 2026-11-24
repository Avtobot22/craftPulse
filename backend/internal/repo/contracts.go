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
	// JoinOrgTx runs fn in the caller's transaction of the organization, or in a new one outside it; only for reads.
	JoinOrgTx(ctx context.Context, orgID entity.ID, fn func(ctx context.Context) error) error
}

type TenantRepo interface {
	CreateTenant(ctx context.Context, orgID entity.ID, at time.Time) error
	NextSeq(ctx context.Context, orgID entity.ID) (int64, error)
	// Clock reads the knowledge counter without moving it; it runs in an organization transaction.
	Clock(ctx context.Context, orgID entity.ID) (lastSeq, reproducibleFrom int64, err error)
	// ActiveTenants reads the registry outside row-level security, for the scheduler of every organization.
	ActiveTenants(ctx context.Context) ([]entity.ID, error)
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
	// PagePersons and PageSourceAccounts read by ID after the cursor, up to the limit of the filter.
	PagePersons(ctx context.Context, orgID entity.ID, f entity.PersonFilter) ([]entity.Person, error)
	CreatePerson(ctx context.Context, orgID entity.ID, name string, kind entity.PersonKind, at time.Time) (entity.ID, error)
	PageSourceAccounts(ctx context.Context, orgID entity.ID, f entity.SourceAccountFilter) ([]entity.AccountView, error)
	// AccountProposals are the people whose other accounts share the login or the email of an account.
	AccountProposals(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID][]entity.AccountProposal, error)
	SourceAccountsByID(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID]entity.AccountView, error)
	// LockCurrentAssignments locks the current links of accounts by ascending ID and returns them by account.
	LockCurrentAssignments(ctx context.Context, orgID entity.ID, accountIDs []entity.ID) (map[entity.ID]entity.AccountAssignment, error)
	// AssignmentsByConfirmation finds the versions saved under confirmation IDs, by confirmation ID.
	AssignmentsByConfirmation(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID]entity.AccountAssignment, error)
	SupersedeAssignments(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertAssignment(ctx context.Context, orgID entity.ID, a entity.AccountAssignment) (entity.ID, error)
	AssignmentAt(ctx context.Context, orgID, accountID entity.ID, seq int64) (entity.AccountAssignment, bool, error)
	UserSourceAccount(ctx context.Context, orgID, userID, instanceID entity.ID) (entity.UserAccount, bool, error)
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
	CurrentComponents(ctx context.Context, orgID entity.ID) ([]entity.Component, error)
	ProjectComponents(ctx context.Context, orgID, projectID entity.ID, at time.Time) ([]entity.Component, error)
	SupersedeComponent(ctx context.Context, orgID, id entity.ID, seq int64, at time.Time) error
	InsertComponent(ctx context.Context, orgID entity.ID, c entity.Component, seq int64) error
	PageAreas(ctx context.Context, orgID entity.ID, kind entity.AreaKind, scope entity.Scope, f entity.AreaFilter,
		p entity.PageRequest, at time.Time) ([]entity.Area, error)
}

// AuthRepo serves login routes, login attempts and sessions outside row-level security and provider
// registrations under it.
type AuthRepo interface {
	// PurgeLoginAttempts and PurgeSessions delete up to limit rows expired by before and report how many went.
	PurgeLoginAttempts(ctx context.Context, before time.Time, limit int) (int64, error)
	PurgeSessions(ctx context.Context, before time.Time, limit int) (int64, error)
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
	WebhookRoute(context.Context, entity.ID) (entity.WebhookRoute, error)
	WebhookRoutes(context.Context, entity.ID, []entity.ID) (map[entity.ID]entity.ID, error)
	EnsureWebhookRoute(context.Context, entity.ID, entity.ID, time.Time) error
	RevokeWebhookRoutes(context.Context, entity.ID, entity.ID, time.Time) error
	SourceConnections(context.Context, entity.ID) ([]entity.SourceConnection, error)
	// PurgeProbes deletes up to limit probes finished by before, except the activated ones, and reports how many went.
	PurgeProbes(ctx context.Context, orgID entity.ID, before time.Time, limit int) (int64, error)
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
	AreaSources(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.AreaSource, error)
	PageSourceAreas(ctx context.Context, orgID entity.ID, f entity.SourceAreaFilter) ([]entity.SourceArea, error)
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
	ParseWebhook(context.Context, entity.AdapterTarget, entity.WebhookInput) (entity.WebhookResult, error)
	AuthorizeURL(context.Context, entity.AdapterTarget, entity.SourceAuthorization) (string, error)
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
	ActivityReceiptConnections(ctx context.Context, orgID entity.ID, receipts []entity.ID) (map[entity.ID]entity.ID, error)
	ActivityTruncations(ctx context.Context, orgID entity.ID, scopes []entity.ID, at time.Time) ([]entity.ActivityHistoryNotice, error)
	ActivityCompleteReceipts(ctx context.Context, orgID entity.ID, receipts []entity.ID) (map[entity.ID]bool, error)
	SaveReceipt(ctx context.Context, r entity.Receipt) error
	Receipt(ctx context.Context, orgID, id entity.ID) (entity.Receipt, error)
	LockReceipt(ctx context.Context, orgID, id entity.ID) (entity.Receipt, error)
	FirstReceiptOfHash(ctx context.Context, orgID entity.ID, hash []byte) (entity.ID, bool, error)
	FirstCIReceiptOfHash(ctx context.Context, orgID, projectID, exclude entity.ID, hash []byte) (entity.ID, bool, error)
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
	RejectAcceptedRecord(ctx context.Context, orgID, receiptID entity.ID, no int, reason string) (bool, error)
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
	ReceiveWebhook(context.Context, entity.ID, entity.ID, time.Time) (entity.ID, error)
	RejectWebhook(context.Context, entity.ID, entity.ID, time.Time, string) (bool, error)
	WebhookStatus(context.Context, entity.ID, entity.ID, time.Time) (*entity.WebhookStatus, error)
	PendingRereads(context.Context, entity.ID, entity.ID) (bool, error)
	State(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.SyncState, bool, error)
	SaveState(ctx context.Context, orgID entity.ID, s entity.SyncState) error
	// AcquireLease takes or renews the lease; false means another holder has it until it runs out.
	AcquireLease(ctx context.Context, orgID, connectionID entity.ID, holder string, until, now time.Time) (bool, error)
	ReleaseLease(ctx context.Context, orgID, connectionID entity.ID, holder string) error
	LeaseHeld(ctx context.Context, orgID, connectionID entity.ID, now time.Time) (bool, error)
	// States lists the states of a connection, or of the organization when connectionID is zero.
	States(ctx context.Context, orgID, connectionID entity.ID) ([]entity.SyncState, error)

	// StartReconcile writes a running reconciliation; false when one of the data kind already runs.
	StartReconcile(ctx context.Context, orgID, connectionID entity.ID, dataKind string, at time.Time) (entity.ID, bool, error)
	RunningReconcile(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.ReconcileRun, bool, error)
	Reconcile(ctx context.Context, orgID, runID entity.ID) (entity.ReconcileRun, bool, error)
	// LastReconcile is the latest reconciliation of a connection, of one data kind when dataKind is set.
	LastReconcile(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.ReconcileRun, bool, error)
	FinishReconcile(ctx context.Context, orgID entity.ID, run entity.ReconcileRun) error
	SaveActivityChecks(ctx context.Context, orgID, runID entity.ID, checks []entity.ActivityHistoryCheck) error
	SaveActivityPagination(ctx context.Context, orgID, scopeID, receiptID entity.ID, dataKind string, at time.Time, complete bool) error
	SaveActivityRecovery(ctx context.Context, orgID entity.ID, recovery *entity.ActivityHistoryRecovery) error
	ActivityChecks(ctx context.Context, orgID entity.ID, scopes []entity.AreaSource, at time.Time) ([]entity.ActivityCheckResult, error)
	ActivityRecoveries(ctx context.Context, orgID entity.ID, scopes []entity.ID, at time.Time) ([]entity.ActivityHistoryRecovery, error)
	ActivityReconciliations(ctx context.Context, orgID entity.ID, connections []entity.ID, from, at time.Time) ([]entity.ReconcileRun, error)

	AddDirtyKeys(ctx context.Context, orgID, connectionID entity.ID, keys []entity.DirtyKey, at time.Time) error
	DirtyKeys(ctx context.Context, orgID, connectionID entity.ID, limit int) ([]entity.DirtyKey, error)
	// DeleteDirtyKeys removes the keys read, except those added again after the read started.
	DeleteDirtyKeys(ctx context.Context, orgID, connectionID entity.ID, hashes [][]byte, readStarted time.Time) error
	CountDirtyKeys(ctx context.Context, orgID, connectionID entity.ID) (int, error)
}

// NormalizeRepo is the normalize schema under row-level security: canonical facts in versions of knowledge.
type NormalizeRepo interface {
	WebhookTargets(context.Context, entity.ID, entity.ID, []entity.ObjectKey) (map[entity.ObjectKey]entity.WebhookMRTarget, error)
	CurrentSyncVersions(context.Context, entity.ID, entity.ID, []string) (map[entity.ObjectKey]entity.SyncObjectVersion, error)
	LinkMRVersions(context.Context, entity.ID, entity.LinkMRFilter) ([]entity.MRVersion, error)
	LinkTaskVersions(context.Context, entity.ID, []entity.ID, []string) ([]entity.TaskVersion, error)
	CurrentRepositoryVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.RepositoryVersion, error)
	KnownRepositoryVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID][]entity.Revision, error)
	SupersedeRepositoryVersions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertRepositoryVersions(ctx context.Context, orgID entity.ID, versions []entity.RepositoryVersion) error
	CurrentMRVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.MRVersion, error)
	KnownMRVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID][]entity.Revision, error)
	SupersedeMRVersions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertMRVersions(ctx context.Context, orgID entity.ID, versions []entity.MRVersion) error
	CurrentMREvents(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) ([]entity.MREvent, error)
	KnownMREvents(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) (map[entity.MRFactKey][]entity.Revision, error)
	SupersedeMREvents(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertMREvents(ctx context.Context, orgID entity.ID, versions []entity.MREvent) error
	CurrentMRRevisions(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) ([]entity.MRRevision, error)
	KnownMRRevisions(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) (map[entity.MRFactKey][]entity.Revision, error)
	SupersedeMRRevisions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertMRRevisions(ctx context.Context, orgID entity.ID, versions []entity.MRRevision) error
	InsertMRDiffs(ctx context.Context, org, area, receipt entity.ID, seq int64, diffs []entity.MRDiffFact) (int64, error)
	ReadMRVersions(ctx context.Context, orgID entity.ID, areas, allowed, mrs []entity.ID, seq int64) ([]entity.MRVersion, error)
	ReadMRRevisions(ctx context.Context, orgID entity.ID, areas, allowed, mrs []entity.ID, seq int64) ([]entity.MRRevision, error)
	ReadMRDiff(ctx context.Context, org, mr entity.ID, areas, allowed []entity.ID, head string, seq int64) (entity.MRDiff, error)
	ReadMREvents(ctx context.Context, orgID entity.ID, areas, allowed, mrs []entity.ID, seq int64) ([]entity.MREvent, error)
	FindMRIDs(ctx context.Context, orgID entity.ID, areas, allowed []entity.ID, key entity.SourceKey, sha string, seq int64) ([]entity.ID, error)
	ReadRepositoryVersion(ctx context.Context, orgID, repositoryID entity.ID) (entity.RepositoryVersion, error)

	InsertRejectedReport(ctx context.Context, orgID entity.ID, report entity.ScanReport, reason string) error
	ReadReportEvidence(ctx context.Context, orgID entity.ID, scope entity.ReportScope) ([]entity.ReportEvidence, error)
	ReadTestExecutions(ctx context.Context, orgID entity.ID, reportIDs []entity.ID, q entity.TestExecutionQuery) ([]entity.ReportExecution, error)
	ReadReportCoverage(ctx context.Context, orgID entity.ID, reportIDs []entity.ID) ([]entity.ReportCoverage, error)
	CurrentCIRunVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.CIRunVersion, error)
	KnownCIRunVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID][]entity.Revision, error)
	SupersedeCIRunVersions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertCIRunVersions(ctx context.Context, orgID entity.ID, versions []entity.CIRunVersion) error
	CurrentCIAttemptVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.CIAttemptVersion, error)
	KnownCIAttemptVersions(ctx context.Context, orgID entity.ID, ids []entity.ID) (map[entity.ID][]entity.Revision, error)
	SupersedeCIAttemptVersions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertCIAttemptVersions(ctx context.Context, orgID entity.ID, versions []entity.CIAttemptVersion) error
	// RunsWithAttempts are the runs among the IDs with a current attempt of a job other than except.
	RunsWithAttempts(ctx context.Context, orgID entity.ID, runIDs []entity.ID, except string) (map[entity.ID]bool, error)
	// MatchRunMRs returns, by the index of each match, the MR that passes it.
	MatchRunMRs(ctx context.Context, orgID, instanceID entity.ID, matches []entity.RunMRMatch) (map[int]entity.ID, error)
	ReadCIRunVersions(ctx context.Context, orgID entity.ID, areas, allowed, runs []entity.ID, seq int64) ([]entity.CIRunVersion, error)
	FindCIRunIDs(ctx context.Context, orgID entity.ID, areas, allowed []entity.ID, q entity.RunsQuery, seq int64) ([]entity.ID, error)
	ReadCIAttemptVersions(ctx context.Context, orgID entity.ID, areas, allowed, runs []entity.ID, seq int64) ([]entity.CIAttemptVersion, error)

	// ReportedRecords are the records of a receipt among nos that already gave a report.
	ReportedRecords(ctx context.Context, orgID, receiptID entity.ID, nos []int) ([]int, error)
	// InsertReport writes a report with its findings, test executions or coverage; the cases of the tests are kept per area.
	InsertReport(ctx context.Context, orgID entity.ID, rep *entity.ScanReport, rows entity.ReportRows) error
	CodeReportAt(ctx context.Context, orgID entity.ID, scope entity.CodeScope, seq int64) (*entity.CodeReport, error)
	DocumentsAt(ctx context.Context, org, area entity.ID, prefix, sha string, seq int64) ([]entity.DocumentVersion, error)
	DocumentationReportsAt(ctx context.Context, org entity.ID, root *entity.DocumentationRoot, sha string, seq int64) ([]entity.DocsReport, []entity.DocsFinding, error)
	ReceiptDocumentationRoots(ctx context.Context, org, receipt entity.ID) ([]entity.DocsReceiptRoot, error)
	ReceiptCodeScopes(ctx context.Context, orgID, receiptID entity.ID) ([]entity.CodeScope, error)
	ReceiptReports(ctx context.Context, orgID, receiptID entity.ID) ([]entity.ReportFindings, error)
	ReportsByID(ctx context.Context, orgID entity.ID, reportIDs []entity.ID) ([]entity.ReportFindings, error)
	LastBranchReports(ctx context.Context, orgID entity.ID, components []entity.ID, refs []string) ([]entity.ReportFindings, error)
	DefaultBranches(ctx context.Context, orgID entity.ID, repositories []entity.ID) (map[entity.ID]string, error)

	AllRepositoryAreas(context.Context, entity.ID) ([]entity.ID, error)
	RepositoryAreas(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.ID, error)
	// EnsureObjects creates the identities that are new with the knowledge number seq and returns all by key.
	EnsureObjects(ctx context.Context, orgID entity.ID, keys []entity.SourceKey, quality entity.IdentityQuality, seq int64) (map[entity.SourceKey]entity.SourceObject, error)
	EnsureAreas(ctx context.Context, orgID entity.ID, areas map[entity.ID]entity.ObjectKind) error
	PlaceObjects(ctx context.Context, orgID entity.ID, placements []entity.ObjectPlacement) error
	// RecordAreaChanges writes the changes of areas at seq and moves their last change to it.
	RecordAreaChanges(ctx context.Context, orgID entity.ID, seq int64, changes []entity.AreaChange, at time.Time) error
	// AreaChangeSeq is the last knowledge number that changed the facts of any of the areas, 0 without one.
	AreaChangeSeq(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) (int64, error)
	AreaChanges(ctx context.Context, orgID, areaID entity.ID, afterSeq int64) ([]entity.AreaChange, error)
	AreaOf(ctx context.Context, orgID, instanceID entity.ID, kind entity.ObjectKind, nativeID string) (entity.ID, error)

	CurrentTaskVersions(ctx context.Context, orgID entity.ID, taskIDs []entity.ID) ([]entity.TaskVersion, error)
	TaskRevisions(ctx context.Context, orgID entity.ID, taskIDs []entity.ID) (map[entity.ID][]entity.Revision, error)
	SupersedeTaskVersions(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertTaskVersions(ctx context.Context, orgID entity.ID, versions []entity.TaskVersion) error
	// TaskVersionsAt and TaskEventsAt skip tasks with their own restriction that are not among allowed.
	TaskVersionsAt(ctx context.Context, orgID, areaID entity.ID, seq int64, allowed []entity.ID) ([]entity.TaskVersion, error)
	CurrentTasksByNative(ctx context.Context, orgID, areaID entity.ID, nativeIDs []string) (map[string]entity.TaskVersion, error)

	CurrentTaskEvents(ctx context.Context, orgID entity.ID, keys []entity.EventKey) ([]entity.TaskEvent, error)
	TaskEventRevisions(ctx context.Context, orgID entity.ID, keys []entity.EventKey) (map[entity.EventKey][]entity.Revision, error)
	SupersedeTaskEvents(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertTaskEvents(ctx context.Context, orgID entity.ID, events []entity.TaskEvent) error
	TaskEventsAt(ctx context.Context, orgID, areaID entity.ID, seq int64, allowed []entity.ID) ([]entity.TaskEvent, error)
	RestrictedObjects(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.RestrictedObject, error)
	// LinkEnds reads the objects at the ends of links with their current area; a deleted object is left out.
	LinkEnds(ctx context.Context, orgID entity.ID, ids []entity.ID) ([]entity.LinkEnd, error)

	CurrentDictionaryEntries(ctx context.Context, orgID entity.ID, keys []entity.DictionaryKey) ([]entity.DictionaryEntry, error)
	DictionaryRevisions(ctx context.Context, orgID entity.ID, keys []entity.DictionaryKey) (map[entity.DictionaryKey][]entity.Revision, error)
	SupersedeDictionaryEntries(ctx context.Context, orgID entity.ID, ids []entity.ID, seq int64, at time.Time) error
	InsertDictionaryEntries(ctx context.Context, orgID entity.ID, entries []entity.DictionaryEntry) error
	Dictionary(ctx context.Context, orgID, areaID entity.ID, kind entity.DictionaryKind) ([]entity.DictionaryEntry, error)
	AreaInstances(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) (map[entity.ID]entity.ID, error)
	AreaProvider(ctx context.Context, orgID, areaID entity.ID) (entity.Provider, error)
	Dictionaries(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.DictionaryEntry, error)
}

// ProcessRepo keeps the immutable versions of the mapping and the calendar of teams (TZ 12.5.6). A version at
// a knowledge number is the last one with known_seq not above it; ErrNotFound when the team has none.
type ProcessRepo interface {
	TimezoneExists(ctx context.Context, name string) (bool, error)
	LastSettingsSeq(ctx context.Context, orgID, teamID entity.ID) (int64, error)

	LatestMappingVersionNo(ctx context.Context, orgID, teamID entity.ID) (int, error)
	MappingVersionAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, error)
	MappingVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.MappingVersion, error)
	PageMappingVersions(ctx context.Context, orgID, teamID entity.ID, p entity.VersionPageRequest) ([]entity.MappingVersion, error)
	InsertMappingVersion(ctx context.Context, orgID entity.ID, v entity.MappingVersion) error

	LatestCalendarVersionNo(ctx context.Context, orgID, teamID entity.ID) (int, error)
	CalendarVersionAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.CalendarVersion, error)
	CalendarVersionByNo(ctx context.Context, orgID, teamID entity.ID, no int) (entity.CalendarVersion, error)
	CalendarVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.CalendarVersion, error)
	PageCalendarVersions(ctx context.Context, orgID, teamID entity.ID, p entity.VersionPageRequest) ([]entity.CalendarVersion, error)
	InsertCalendarVersion(ctx context.Context, orgID entity.ID, v entity.CalendarVersion) error

	LatestTargetVersionNo(ctx context.Context, orgID, teamID entity.ID, g entity.TargetGroup) (int, error)
	CurrentTargetVersion(ctx context.Context, orgID, teamID entity.ID, g entity.TargetGroup) (entity.TargetVersion, error)
	TargetVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.TargetVersion, error)
	PageTargetVersions(ctx context.Context, orgID, teamID entity.ID, f entity.TargetVersionFilter) ([]entity.TargetVersion, error)
	TargetVersionsAt(ctx context.Context, orgID, teamID entity.ID, seq int64) ([]entity.TargetVersion, error)
	InsertTargetVersion(ctx context.Context, orgID entity.ID, v entity.TargetVersion) error
}

// CalcRepo keeps the series of calc, their immutable revisions and the parts they share (TZ 12.5.8, 12.7).
// Every call runs in an organization transaction; a missing series or revision is entity.ErrNotFound.
type CalcRepo interface {
	// EnsureSeries creates the series of the scope, set and step unless it exists, and returns it.
	EnsureSeries(ctx context.Context, orgID entity.ID, s entity.CalcSeries) (entity.CalcSeries, error)
	Series(ctx context.Context, orgID, id entity.ID) (entity.CalcSeries, error)
	// OrgSeries are the series of a set of metrics of the organization.
	OrgSeries(ctx context.Context, orgID entity.ID, metricSet string) ([]entity.CalcSeries, error)
	// LastRevision is the revision of the series with the highest number; false before the first one.
	LastRevision(ctx context.Context, orgID, seriesID entity.ID) (entity.CalcRevision, bool, error)
	// RevisionAfter is the first revision of the series numbered after afterNo; false when there is none yet.
	RevisionAfter(ctx context.Context, orgID, seriesID entity.ID, afterNo int) (entity.CalcRevision, bool, error)
	// PartByHash finds a stored part by the hash of its input.
	PartByHash(ctx context.Context, orgID entity.ID, inputHash []byte) (entity.ID, bool, error)
	FailStaleRevisions(ctx context.Context, orgID, seriesID entity.ID, before time.Time) error
	// CreateRevision writes a building revision with the next number; false when the series already builds one.
	CreateRevision(ctx context.Context, orgID entity.ID, r entity.CalcRevision) (bool, error)
	// SetRevisionPrevious names the revision a building one replaces.
	SetRevisionPrevious(ctx context.Context, orgID, revisionID entity.ID, prev *entity.ID) error
	InsertRevisionInputs(ctx context.Context, orgID, revisionID entity.ID, inputs []entity.RevisionInput) error
	Revision(ctx context.Context, orgID, id entity.ID) (entity.CalcRevision, error)
	// TouchRevision, FailRevision and PublishRevision change a building revision only; false when it is not one.
	TouchRevision(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error)
	FailRevision(ctx context.Context, orgID, id entity.ID) (bool, error)
	PublishRevision(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error)
	SupersedeRevision(ctx context.Context, orgID, id entity.ID, at time.Time) error
	// MoveCurrentRevision moves the pointer of the series from prev; false when another revision holds it.
	MoveCurrentRevision(ctx context.Context, orgID, seriesID, revisionID entity.ID, prev *entity.ID, seq int64) (bool, error)
	// SavePart stores a part under the new ID unless one with its input hash exists; it returns the ID of the
	// part kept and whether it is new. The row of an existing part stays locked until the commit.
	SavePart(ctx context.Context, orgID, newID entity.ID, p *entity.StoredPart, at time.Time) (entity.ID, bool, error)
	LinkPart(ctx context.Context, orgID, revisionID entity.ID, partNo int, partID entity.ID) error
	RevisionPoints(ctx context.Context, orgID, revisionID entity.ID) ([]entity.SeriesPoint, error)
	// ScopeSeries are the series of a scope and a set of metrics, one per step.
	ScopeSeries(ctx context.Context, orgID entity.ID, scopeHash []byte, metricSet string) ([]entity.CalcSeries, error)
	// PointSamples reads the contributions of the points of a metric and an estimator lying within the window,
	// with the type and the size of their tasks.
	PointSamples(ctx context.Context, orgID, revisionID entity.ID, metricID, estimator string, w entity.Window) ([]entity.PointSample, error)
	PointContributions(ctx context.Context, orgID, revisionID entity.ID, q entity.ContributionQuery) ([]entity.Contribution, error)
	TaskIntervals(ctx context.Context, orgID, revisionID, taskID entity.ID) ([]entity.WorkflowInterval, error)
	// TasksIntervals are the intervals of several tasks of a revision, ordered by task.
	TasksIntervals(ctx context.Context, orgID, revisionID entity.ID, taskIDs []entity.ID) ([]entity.WorkflowInterval, error)
	// RevisionCohort reads every contribution of a revision with the attributes of its task.
	RevisionCohort(ctx context.Context, orgID, revisionID entity.ID) ([]entity.CohortRow, error)
	RevisionInputs(ctx context.Context, orgID, revisionID entity.ID) ([]entity.RevisionInput, error)
	MarkBasisDeleted(ctx context.Context, orgID, objectID entity.ID) (int64, error)
	MarkBasisExpired(ctx context.Context, orgID entity.ID, reproducibleFrom int64) (int64, error)
	// MarkDefinitionRetired marks revisions computed with a definition missing from current, "ID@version".
	MarkDefinitionRetired(ctx context.Context, orgID entity.ID, current []string) (int64, error)
	// UnusedParts locks up to limit parts after the cursor that no live revision refers to, created before before.
	UnusedParts(ctx context.Context, orgID, after entity.ID, before time.Time, limit int) ([]entity.ID, error)
	DeleteParts(ctx context.Context, orgID entity.ID, ids []entity.ID) error
	CreateSavedView(ctx context.Context, v entity.SavedView) error
	SavedView(ctx context.Context, orgID, id entity.ID) (entity.SavedView, error)
	PageSavedViews(ctx context.Context, orgID, ownerID entity.ID, q entity.SavedViewQuery) ([]entity.SavedView, error)
	// DeleteSavedView deletes only the owner's view; a missing view succeeds, another owner's is not found.
	DeleteSavedView(ctx context.Context, orgID, ownerID, id entity.ID) error
	// PurgeRevisions deletes at most limit superseded revisions before the cutoff without any holder.
	PurgeRevisions(ctx context.Context, orgID entity.ID, before, at time.Time, limit int) (int64, error)
}

// ViewRepo keeps the public descriptors of runs users asked for (TZ 12.5.8).
type ViewRepo interface {
	// CreateOperation writes a queued descriptor; false when the same user has an active one of the resource.
	CreateOperation(ctx context.Context, op entity.Operation) (bool, error)
	// ActiveOperation locks the active descriptor of a user for the resource of op.
	ActiveOperation(ctx context.Context, op entity.Operation) (entity.Operation, bool, error)
	Operation(ctx context.Context, orgID, id entity.ID) (entity.Operation, bool, error)
	// UpdateOperation moves an active descriptor to its new status; a finished one does not change.
	UpdateOperation(ctx context.Context, op entity.Operation) error
	// PurgeOperations deletes up to limit operations completed by before and reports how many went.
	PurgeOperations(ctx context.Context, orgID entity.ID, before time.Time, limit int) (int64, error)
}

// ChangeRepo keeps the versions of the matrix of checks of projects.
type ChangeRepo interface {
	ExpectedPipelineReports(ctx context.Context, orgID, areaID entity.ID, at time.Time, whole bool, projects []entity.ID) ([]entity.ExpectedReport, error)
	LatestProfileVersion(ctx context.Context, orgID, projectID entity.ID) (entity.CheckProfileVersion, error)
	ProfileVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.CheckProfileVersion, error)
	InsertProfileVersion(ctx context.Context, orgID entity.ID, v entity.CheckProfileVersion) error

	IdentitiesByFingerprint(ctx context.Context, orgID, areaID entity.ID, method int, fingerprints [][]byte) (map[string]entity.ID, error)
	IdentitiesByContent(ctx context.Context, orgID, areaID entity.ID, method int, contents [][]byte) ([]entity.FindingIdentity, error)
	InsertFindingIdentity(ctx context.Context, orgID entity.ID, f *entity.FindingIdentity) (entity.ID, error)
	AssignedOccurrences(ctx context.Context, orgID entity.ID, occurrences []entity.ID) ([]entity.ID, error)
	InsertOccurrenceIdentities(ctx context.Context, orgID entity.ID, list []entity.OccurrenceIdentity, at time.Time, seq int64) error
	FindingIdentity(ctx context.Context, orgID, id entity.ID) (entity.FindingIdentity, error)
	FindingKindsForException(ctx context.Context, org entity.ID, s entity.ExceptionScope) ([]string, error)
	LockExceptionProjection(ctx context.Context, org, area entity.ID) error
	ProjectExceptions(ctx context.Context, org, area entity.ID, seq int64, at time.Time, versions []entity.ID) error
	OccurrenceIdentities(ctx context.Context, orgID entity.ID, occurrences []entity.ID) ([]entity.OccurrenceIdentity, error)
	LockCurrentBaseline(ctx context.Context, orgID, componentID entity.ID, language, kind string) (entity.Baseline, error)
	ReplaceBaseline(ctx context.Context, orgID entity.ID, old, b *entity.Baseline) error
	CurrentBaselines(ctx context.Context, orgID entity.ID, components []entity.ID) ([]entity.Baseline, error)
}

// DecisionRepo owns the journal and the next scheduled boundary of each exception.
type DecisionRepo interface {
	LockConfirmation(ctx context.Context, org, key entity.ID) error
	InsertDecision(ctx context.Context, org, actor entity.ID, at time.Time) (entity.ID, error)
	Decision(ctx context.Context, org, id entity.ID, lock bool) (entity.ExceptionRecord, error)
	SetWake(ctx context.Context, org, id entity.ID, at *time.Time) error
	VersionByConfirmation(ctx context.Context, org, key entity.ID) (entity.ExceptionVersion, error)
	TransitionByConfirmation(ctx context.Context, org, key entity.ID) (entity.ID, entity.ExceptionTransition, error)
	InsertVersion(ctx context.Context, org entity.ID, v *entity.ExceptionVersion) (entity.ID, error)
	InsertTransition(ctx context.Context, org entity.ID, t *entity.ExceptionTransition) error
	Histories(ctx context.Context, org entity.ID, ids []entity.ID, seq int64) (map[entity.ID]*entity.ExceptionRecord, error)
	AreaDecisionIDs(ctx context.Context, org, area entity.ID, seq int64, at time.Time, includeExpired bool) ([]entity.ID, error)
	PageDecisions(ctx context.Context, org entity.ID, q *entity.ExceptionQuery, cut *entity.ExceptionCursor, limit int32) ([]entity.ExceptionCursor, error)
}
