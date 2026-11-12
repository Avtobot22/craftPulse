package entity

import (
	"fmt"
	"time"
)

type CheckKind string

const (
	CheckSite             CheckKind = "site"
	CheckAdapter          CheckKind = "adapter"
	CheckAdapterStore     CheckKind = "adapter_store"
	CheckIdentityProvider CheckKind = "identity_provider"
	CheckUpdateSource     CheckKind = "update_source"
	CheckDatabase         CheckKind = "database"
	CheckObjectStore      CheckKind = "object_store"
	CheckRunnerNetwork    CheckKind = "runner_network"
)

type CheckOutcome string

const (
	CheckPassed  CheckOutcome = "passed"
	CheckFailed  CheckOutcome = "failed"
	CheckSkipped CheckOutcome = "skipped"
)

type ProtocolOutcome string

const (
	ProtocolRunning ProtocolOutcome = "running"
	ProtocolPassed  ProtocolOutcome = "passed"
	ProtocolFailed  ProtocolOutcome = "failed"
)

// Reason classes of a failed or skipped check; an unreachable provider names one of the first four (UC-01 A3).
const (
	ReasonAddress           = "address"
	ReasonCertificate       = "certificate"
	ReasonNetworkScope      = "network_scope"
	ReasonRegistration      = "registration"
	ReasonUnavailable       = "unavailable"
	ReasonLocaleUnsupported = "locale_unsupported"
	ReasonExtensionMissing  = "extension_missing"
	ReasonSchemaOutdated    = "schema_outdated"
	ReasonNotConfigured     = "not_configured"
)

// CheckFailure is the error a probe returns when the checked component is not usable.
type CheckFailure struct {
	Reason string
	Detail string
}

func (f *CheckFailure) Error() string { return fmt.Sprintf("%s: %s", f.Reason, f.Detail) }

type CheckResult struct {
	Kind    CheckKind
	Outcome CheckOutcome
	Reason  string
	Detail  string
}

type DatabaseFacts struct {
	LocaleProvider string
	Locale         string
	HasBtreeGist   bool
	SchemaVersion  int64
	ExpectedSchema int64
}

type InstallParams struct {
	ReleaseSHA    string
	Images        map[string]string
	SiteURL       string
	Organizations []NewOrganization
	RunnerNetwork []string
}

type AppliedOrganization struct {
	Slug    string
	ID      ID
	Created bool
}

type Protocol struct {
	ID         ID
	StartedAt  time.Time
	FinishedAt time.Time
	ReleaseSHA string
	Outcome    ProtocolOutcome
	Checks     []CheckResult
}
