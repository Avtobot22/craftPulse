package entity

const (
	MetricSetSecurity                 = "security"
	ObjectSecurityFinding  ObjectKind = "security_finding"
	ObjectAffectedPackage  ObjectKind = "affected_package"
	ObjectSecurityAdvisory ObjectKind = "security_advisory"
)

const (
	DependencyDeltaComputed    = "computed"
	DependencyDeltaNoBase      = "no_base"
	DependencyDeltaUnavailable = "unavailable"
	DependencyAdded            = "added"
	DependencyChanged          = "changed"

	DependencyFixable    = "fixable"
	DependencyNoFix      = "no_fix"
	DependencyFixUnknown = "unknown"

	SecretScopeHead = "head"
	SecretScopeDiff = "diff"
)

type SecurityScope struct {
	AreaID      ID
	ComponentID *ID
	SHA         string
	KnownSeq    int64
}

type SecurityFacts struct {
	Scope    SecurityScope
	Reports  []ReportEvidence
	Findings []SecurityFinding
}

type SecurityFinding struct {
	ID       ID
	ReportID ID
	FindingOccurrence
}

// DependencyEvidence describes the affected package in its lockfile context.
type DependencyEvidence struct {
	Name         string `json:"name"`
	Version      string `json:"version,omitempty"`
	FixedVersion string `json:"fixed_version,omitempty"`
	FixState     string `json:"fix_state,omitempty"`
	Ecosystem    string `json:"ecosystem,omitempty"`
	Context      string `json:"context,omitempty"`
	PackagePath  string `json:"package_path,omitempty"`
	Relationship string `json:"relationship,omitempty"`
	Dev          *bool  `json:"dev,omitempty"`
	Delta        string `json:"delta,omitempty"`
}

type AdvisoryEvidence struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases,omitempty"`
}

type SecurityEvidence struct {
	CWE                      []string            `json:"cwe,omitempty"`
	Package                  *DependencyEvidence `json:"package,omitempty"`
	Advisory                 *AdvisoryEvidence   `json:"advisory,omitempty"`
	Resource                 string              `json:"resource,omitempty"`
	Suppressed               bool                `json:"suppressed,omitempty"`
	MappedConfidence         string              `json:"mapped_confidence,omitempty"`
	ConfidenceMappingVersion int                 `json:"confidence_mapping_version,omitempty"`
}

type SecurityRevision struct {
	Scope       SecurityScope
	Definitions []RevisionInput
	Reports     []ReportEvidence
	Metrics     map[string]SecurityMetric
}

// SecurityMetric has no value when the corresponding scan or denominator is unavailable.
type SecurityMetric struct {
	State         DataState
	ReasonCode    string
	Value         *float64
	Numerator     *int
	Denominator   *int
	Groups        []SecurityGroup
	Contributions []SecurityContribution
}

type SecurityGroup struct {
	Kind           string
	NativeSeverity string
	RuleID         string
	CWE            string
	Scope          string
	State          DataState
	ReasonCode     string
	Count          *int
}

type SecurityContribution struct {
	Role       ContributionRole
	ReasonCode string
	Findings   []SecurityFinding
	Package    *AffectedPackage
	AdvisoryID string
	Scope      string
}

type AffectedPackage struct {
	DependencyEvidence
	PURL     string
	Lockfile string
}
