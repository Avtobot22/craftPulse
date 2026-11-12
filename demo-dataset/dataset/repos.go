package dataset

// Repository model of the demo set: Git history with reproducible SHAs, Git documents, injected defects of
// demo steps 5-7 and the patches applied during the demo. File contents live in the bare repositories and
// patches written by RepoSet.Write, the model keeps only paths, lines and hashes.

const (
	FormatMarkdown = "markdown"
	FormatOpenAPI  = "openapi"

	BrokenFile   = "file"
	BrokenAnchor = "anchor"

	DefectTest     = "test"
	DefectLint     = "lint"
	DefectSecurity = "security"
)

type RepoFile struct {
	SchemaVersion string       `json:"schema_version"`
	Seed          uint64       `json:"seed"`
	Period        Window       `json:"period"`
	AsOf          string       `json:"as_of"`
	Repositories  []Repository `json:"repositories"`
	Patches       []Patch      `json:"patches"`
}

type Repository struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	InstanceID     string   `json:"instance_id"`
	AreaID         string   `json:"area_id"`
	Path           string   `json:"path"`
	BarePath       string   `json:"bare_path"`
	DefaultBranch  string   `json:"default_branch"`
	Restricted     bool     `json:"restricted"`
	Language       string   `json:"language"`
	Head           string   `json:"head"`
	HeadTree       string   `json:"head_tree"`
	Lines          int      `json:"lines"`
	DocRoots       []string `json:"doc_roots"`
	Commits        []Commit `json:"commits"`
	// Documents lists every file under the doc roots with its local links.
	Documents        []GitDocument `json:"documents"`
	UnownedDocuments []string      `json:"unowned_documents"`
	BrokenLinks      []BrokenLink  `json:"broken_links"`
	InvalidSpecs     []string      `json:"invalid_specs"`
	Defects          []Defect      `json:"defects"`
}

// Commit keeps everything its SHA depends on except the file contents, which the tree SHA covers.
type Commit struct {
	SHA         string     `json:"sha"`
	Tree        string     `json:"tree"`
	Parents     []string   `json:"parents"`
	AuthorID    string     `json:"author_id"`
	AuthorTime  SourceTime `json:"author_time"`
	CommitterID string     `json:"committer_id"`
	CommitTime  SourceTime `json:"commit_time"`
	Message     string     `json:"message"`
	Files       []string   `json:"files"`
}

// GitDocument is a document under a doc root. Owner is empty when the document names none; SpecValid is
// set only for OpenAPI specifications.
type GitDocument struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Format    string    `json:"format"`
	Owner     string    `json:"owner"`
	SpecValid *bool     `json:"spec_valid"`
	Anchors   []string  `json:"anchors"`
	Links     []DocLink `json:"links"`
}

// DocLink is a local Markdown link; Broken is empty, BrokenFile or BrokenAnchor.
type DocLink struct {
	Line   int    `json:"line"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Anchor string `json:"anchor"`
	Broken string `json:"broken"`
}

type BrokenLink struct {
	ID       string `json:"id"`
	Document string `json:"document"`
	Line     int    `json:"line"`
	Target   string `json:"target"`
	Kind     string `json:"kind"`
}

// Defect is an injected finding. Exception holds the local suppression on the same line, FixedByStep the
// demo step whose patch removes the defect, zero when no patch does.
type Defect struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Tool        string `json:"tool"`
	Rule        string `json:"rule"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Exception   string `json:"exception"`
	FixedByStep int    `json:"fixed_by_step"`
}

// Patch is applied to Base during the demo; ResultTree is the tree after git apply on Base.
type Patch struct {
	ID           string   `json:"id"`
	Step         int      `json:"step"`
	RepositoryID string   `json:"repository_id"`
	Path         string   `json:"path"`
	SHA256       string   `json:"sha256"`
	Base         string   `json:"base"`
	ResultTree   string   `json:"result_tree"`
	Files        []string `json:"files"`
	Fixes        []string `json:"fixes"`
}

// Objects is the per-kind object file of repositories and Git documents for the manifest command.
func (f *RepoFile) Objects() ObjectFile {
	var objects []ModelObject
	for _, repo := range f.Repositories {
		objects = append(objects, ModelObject{ID: repo.ID, OrganizationID: repo.OrganizationID, Kind: "repository"})
		for _, doc := range repo.Documents {
			objects = append(objects, ModelObject{ID: doc.ID, OrganizationID: repo.OrganizationID, Kind: "git_document"})
		}
	}
	return ObjectFile{SchemaVersion: "1", Objects: objects, Variants: []VariantObjects{}}
}
