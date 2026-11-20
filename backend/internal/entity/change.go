package entity

// Modes of a row of the matrix of checks: a ready report of the client or a builtin profile of craftPulse.
const (
	CheckModeImport  = "import"
	CheckModeBuiltin = "builtin"
)

// Analysis kinds a matrix row and a report name.
const (
	AnalysisCode     = "code"
	AnalysisSAST     = "sast"
	AnalysisSCA      = "sca"
	AnalysisSecrets  = "secrets"
	AnalysisIaC      = "iac"
	AnalysisImage    = "image"
	AnalysisJUnit    = "junit"
	AnalysisCoverage = "coverage"
	AnalysisDocs     = "docs"
	AnalysisOpenAPI  = "openapi"
	AnalysisSonar    = "sonar"
)

// Reasons a component or a language of it stays unchecked (FR-035 A4).
const (
	ReasonProfileMissing     = "profile_missing"
	ReasonProfileUnavailable = "profile_unavailable"
)

// CIGitLab is the CI system the configuration snippet is generated for on M0.
const CIGitLab = "gitlab_ci"

// CheckProfile is a profile of the catalog of a release: a builtin one runs from its image by digest, an import
// one takes a ready report of the client and has no image. An empty Languages list means a check without a
// language; AnyLanguage accepts every language.
type CheckProfile struct {
	ID            string
	Mode          string
	Languages     []string
	AnyLanguage   bool
	AnalysisKinds []string
	Image         string
	Digest        string
	ToolVersions  map[string]string
	RulesHash     []byte
	// Script is the commands of the job of a builtin profile in the CI of the client.
	Script []string
	// ReportFormat is the format an import profile reads; empty when the data comes from an adapter.
	ReportFormat string
}

// CheckCatalog is the catalog of profiles of a release and the image of the step that uploads imports.
type CheckCatalog struct {
	StepImage  string
	StepDigest string
	Profiles   []CheckProfile
}

// ProfileEntry is a row of the matrix: one per component, language and analysis kind.
type ProfileEntry struct {
	ComponentID  ID
	Language     string
	AnalysisKind string
	Mode         string
	ProfileID    string
	ToolVersions map[string]string
	RulesHash    []byte
}

type CheckProfileDraft struct {
	Entries []ProfileEntry
}

// Uncovered is a component or a language of it no profile checks.
type Uncovered struct {
	ComponentID ID
	Language    string
	ReasonCode  string
}

// CheckProfileVersion is a confirmed version of the matrix of checks of a project (FR-035).
type CheckProfileVersion struct {
	ID            ID
	ProjectID     ID
	VersionNo     int
	BaseVersionNo *int
	ContentHash   []byte
	KnownSeq      int64
	Confirmation
	Entries   []ProfileEntry
	Uncovered []Uncovered
}

// CIProfileEntry is a row of the matrix as the step of CI reads it.
type CIProfileEntry struct {
	ComponentID  ID
	PathPrefix   string
	Language     string
	AnalysisKind string
	Mode         string
	ProfileID    string
	ImageDigest  string
}

// CIDocumentationRoot is a confirmed Git documentation root of a repository of the project with the analysis
// kind of the matrix that checks it.
type CIDocumentationRoot struct {
	RootID     ID
	Kind       string
	PathPrefix string
}

// CICheckProfile is what craftpulse-ci reads by the upload token of a project.
type CICheckProfile struct {
	ProjectID          ID
	VersionNo          int
	Entries            []CIProfileEntry
	ExpectedReports    []string
	DocumentationRoots []CIDocumentationRoot
	Limits             PackageLimits
}

// PackageLimits are the bounds of a package of CI (NFR-015).
type PackageLimits struct {
	MaxPackageBytes  int64
	MaxUnpackedBytes int64
	MaxFiles         int
}

// CIVariable is a variable the snippet uses; a masked one holds a secret set in the CI of the client.
type CIVariable struct {
	Name   string
	Masked bool
}

type CIConfiguration struct {
	System    string
	VersionNo int
	Snippet   string
	Variables []CIVariable
}
