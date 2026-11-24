package entity

const (
	MRDiffComplete = "complete"
	MRDiffPartial  = "partial"
)

type MRDiff struct {
	MRID         ID
	RevisionID   ID
	RevisionKey  string
	HeadSHA      string
	BaseSHA      string
	KnownSeq     int64
	Completeness string
	Files        []MRDiffFile
}

type MRDiffFact struct {
	MRDiff
	Record      RecordRef
	ContentHash []byte
}

type MRDiffFile struct {
	OldPath      string        `json:"old_path"`
	NewPath      string        `json:"new_path"`
	NewFile      bool          `json:"new_file"`
	DeletedFile  bool          `json:"deleted_file"`
	RenamedFile  bool          `json:"renamed_file"`
	Binary       bool          `json:"binary"`
	AddedLines   *int          `json:"added_lines"`
	DeletedLines *int          `json:"deleted_lines"`
	HeadRanges   []MRDiffRange `json:"head_ranges"`
}

type MRDiffRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// MRChangedLines contains the coverage evidence of one file at the requested head.
type MRChangedLines struct {
	File         MRDiffFile
	Completeness string
	RevisionID   ID
	BaseSHA      string
	HeadSHA      string
}
