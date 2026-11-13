package envelope

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
)

// Package layout.
const (
	ManifestName = "manifest.json"
	RecordsName  = "records.jsonl"
	TasksCSVName = "records.csv"
	FilesDir     = "files/"
)

// Reasons for rejecting the whole package.
const (
	ReasonInvalidArchive           = "invalid_archive"
	ReasonManifestNotFirst         = "manifest_not_first"
	ReasonInvalidManifest          = "invalid_manifest"
	ReasonUnsupportedSchema        = "unsupported_schema"
	ReasonUnsupportedSchemaVersion = "unsupported_schema_version"
	ReasonUnsafePath               = "unsafe_path"
	ReasonUnexpectedEntry          = "unexpected_entry"
	ReasonDuplicateEntry           = "duplicate_entry"
	ReasonLimitExceeded            = "limit_exceeded"
	ReasonInvalidCSV               = "invalid_csv"
	ReasonUnsafeXML                = "unsafe_xml"
)

// Reasons for rejecting one record.
const (
	ReasonInvalidRecord            = "invalid_record"
	ReasonMissingNativeID          = "missing_native_id"
	ReasonMissingEventTime         = "missing_event_time"
	ReasonKindNotAllowed           = "kind_not_allowed"
	ReasonUnsupportedPayloadSchema = "unsupported_payload_schema"
	ReasonClassNotAllowed          = "class_not_allowed"
	ReasonNotSnapshot              = "not_snapshot"
	ReasonMissingReadID            = "missing_read_id"
	ReasonSchemaViolation          = "schema_violation"
	ReasonMissingFile              = "missing_file"
	ReasonFileMismatch             = "file_mismatch"
)

const (
	_maxManifestBytes = 1 << 20
	_maxRecordBytes   = 4 << 20
	_maxTailBytes     = 64 << 10
)

// Limits bound a package before it is unpacked (NFR-015).
type Limits struct {
	MaxPackageBytes  int64
	MaxUnpackedBytes int64
	MaxFiles         int
}

// DefaultLimits are the NFR-015 limits: 25 MiB packed, 100 MiB unpacked, 1 000 files.
var DefaultLimits = Limits{MaxPackageBytes: 25 << 20, MaxUnpackedBytes: 100 << 20, MaxFiles: 1000}

// Rejection is a stable reason code with a detail that names the field or entry, never its value.
type Rejection struct {
	Code   string
	Detail string
}

func (r *Rejection) Error() string {
	if r.Detail == "" {
		return r.Code
	}
	return r.Code + ": " + r.Detail
}

// RecordResult is the outcome of one record; No counts lines of records.jsonl or data rows of records.csv.
type RecordResult struct {
	No             int
	Class          string
	Kind           string
	PayloadSchema  string
	NativeNS       string
	NativeID       string
	SourceRevision *string
	Hash           string
	Rejection      *Rejection
}

// Result of Check. Envelope is set when the whole package is rejected; Records are then not reported.
type Result struct {
	Manifest *Manifest
	Envelope *Rejection
	Records  []RecordResult
	// ContentHash is set by Clean: the SHA-256 of the cleaned package with created_at of the manifest
	// left out, so a page read again or a file sent again gives the same one.
	ContentHash []byte
}

// Rejected counts rejected records.
func (r *Result) Rejected() int {
	n := 0
	for _, rec := range r.Records {
		if rec.Rejection != nil {
			n++
		}
	}
	return n
}

type fileEntry struct {
	sha256 string
	size   int64
}

type checker struct {
	// lines keeps the record of every accepted number when the package is cleaned.
	lines      map[int][]byte
	limits     Limits
	manifest   map[string]any
	rule       packageRule
	result     Result
	files      map[string]fileEntry
	entries    map[string]bool
	unpacked   int64
	fileCount  int
	fileRefs   []pendingFile
	hasRecords bool
}

// pendingFile is a file reference checked after the whole archive is read; verify compares hash and size.
type pendingFile struct {
	result       int
	path, sha256 string
	size         int64
	verify       bool
}

// Check reads a package as a stream and validates the manifest, every entry and every record.
func Check(r io.Reader, limits Limits) *Result {
	c := &checker{limits: limits, files: map[string]fileEntry{}, entries: map[string]bool{}}
	counted := &countingReader{r: r, limit: limits.MaxPackageBytes}
	if rejection := c.read(counted); rejection != nil {
		if counted.exceeded {
			rejection = &Rejection{ReasonLimitExceeded, "package is larger than the limit"}
		}
		return &Result{Manifest: c.result.Manifest, Envelope: rejection}
	}
	c.checkFiles()
	return &c.result
}

func (c *checker) read(r io.Reader) *Rejection {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return &Rejection{ReasonInvalidArchive, "not a gzip stream"}
	}
	tr := tar.NewReader(gz)
	header, err := tr.Next()
	if err != nil {
		return &Rejection{ReasonInvalidArchive, "empty or broken tar archive"}
	}
	if header.Name != ManifestName || header.Typeflag != tar.TypeReg {
		return &Rejection{ReasonManifestNotFirst, "first entry must be " + ManifestName}
	}
	if rejection := c.readManifest(tr, header); rejection != nil {
		return rejection
	}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return &Rejection{ReasonInvalidArchive, "broken tar archive"}
		}
		if rejection := c.readEntry(tr, header); rejection != nil {
			return rejection
		}
	}
	// Bytes after the end of the tar archive are padding; a long tail is not a package.
	tail, err := io.Copy(io.Discard, io.LimitReader(gz, _maxTailBytes+1))
	if err != nil {
		return &Rejection{ReasonInvalidArchive, "broken gzip stream"}
	}
	if tail > _maxTailBytes {
		return &Rejection{ReasonInvalidArchive, "data after the end of the tar archive"}
	}
	return nil
}

func (c *checker) readManifest(tr *tar.Reader, header *tar.Header) *Rejection {
	if header.Size > _maxManifestBytes {
		return &Rejection{ReasonLimitExceeded, ManifestName + " is larger than 1 MiB"}
	}
	if rejection := c.count(header); rejection != nil {
		return rejection
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		return &Rejection{ReasonInvalidArchive, "cannot read " + ManifestName}
	}
	value, err := decode(data)
	if err != nil {
		return &Rejection{ReasonInvalidManifest, "manifest is not a JSON value"}
	}
	if bad := _rules.Manifest.validate(value, ""); bad != nil {
		return &Rejection{ReasonInvalidManifest, bad.Error()}
	}
	c.manifest, _ = value.(map[string]any) // the manifest schema requires an object
	rule, ok := _rules.Packages[text(c.manifest, "schema_id")]
	if !ok {
		return &Rejection{ReasonUnsupportedSchema, "schema_id is not supported"}
	}
	if !slices.Contains(rule.Versions, text(c.manifest, "schema_version")) {
		return &Rejection{ReasonUnsupportedSchemaVersion, "schema_version is not supported for this schema_id"}
	}
	if c.snapshot() && c.manifest["read_id"] == nil {
		return &Rejection{ReasonInvalidManifest, "read_id: required for a snapshot"}
	}
	var typed Manifest
	if err := json.Unmarshal(data, &typed); err != nil {
		return &Rejection{ReasonInvalidManifest, "manifest does not decode"}
	}
	c.rule = rule
	c.result.Manifest = &typed
	return nil
}

func (c *checker) readEntry(tr *tar.Reader, header *tar.Header) *Rejection {
	name := header.Name
	if !safePath(name) {
		return &Rejection{ReasonUnsafePath, "entry path is not a clean relative path"}
	}
	if header.Typeflag == tar.TypeDir && strings.HasPrefix(name+"/", FilesDir) {
		return nil
	}
	if header.Typeflag != tar.TypeReg {
		return &Rejection{ReasonUnsafePath, name + " is not a regular file"}
	}
	if c.entries[name] {
		return &Rejection{ReasonDuplicateEntry, name + " appears twice"}
	}
	c.entries[name] = true
	if rejection := c.count(header); rejection != nil {
		return rejection
	}
	switch {
	case name == RecordsName && !c.hasRecords:
		c.hasRecords = true
		return c.readRecords(tr)
	case name == TasksCSVName && !c.hasRecords && c.snapshot() && c.manifest["schema_id"] == "tasks":
		c.hasRecords = true
		return c.readTasksCSV(tr)
	case strings.HasPrefix(name, FilesDir):
		h := sha256.New()
		guard := &xmlGuard{r: tr}
		size, err := io.Copy(h, guard)
		if err != nil {
			return &Rejection{ReasonInvalidArchive, "cannot read " + name}
		}
		if guard.found {
			return &Rejection{ReasonUnsafeXML, name + " declares a document type or an entity"}
		}
		c.files[name] = fileEntry{sha256: hex.EncodeToString(h.Sum(nil)), size: size}
		return nil
	case name == TasksCSVName && !c.hasRecords:
		return &Rejection{ReasonUnexpectedEntry, TasksCSVName + " is allowed only in a tasks snapshot"}
	case name == RecordsName || name == TasksCSVName:
		return &Rejection{ReasonDuplicateEntry, "package has more than one record file"}
	}
	return &Rejection{ReasonUnexpectedEntry, name + " is not part of the package layout"}
}

func (c *checker) count(header *tar.Header) *Rejection {
	c.unpacked += header.Size
	c.fileCount++
	if c.fileCount > c.limits.MaxFiles {
		return &Rejection{ReasonLimitExceeded, "too many files"}
	}
	if c.unpacked > c.limits.MaxUnpackedBytes {
		return &Rejection{ReasonLimitExceeded, "unpacked size is larger than the limit"}
	}
	return nil
}

func (c *checker) readRecords(r io.Reader) *Rejection {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), _maxRecordBytes)
	no := 0
	for scanner.Scan() {
		no++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		c.checkRecord(no, line)
	}
	if err := scanner.Err(); err != nil {
		return &Rejection{ReasonInvalidArchive, fmt.Sprintf("%s line %d is unreadable or larger than 4 MiB", RecordsName, no+1)}
	}
	return nil
}

func (c *checker) snapshot() bool {
	snapshot, _ := c.manifest["snapshot"].(bool) // validated as boolean by the manifest schema
	return snapshot
}

// checkFiles resolves file references once every entry of the archive is known.
func (c *checker) checkFiles() {
	for _, ref := range c.fileRefs {
		rec := &c.result.Records[ref.result]
		if rec.Rejection != nil {
			continue
		}
		file, ok := c.files[ref.path]
		switch {
		case !ok:
			rec.Rejection = &Rejection{ReasonMissingFile, ref.path + " is not in the package"}
			rec.Hash = ""
		case ref.verify && (file.sha256 != ref.sha256 || file.size != ref.size):
			rec.Rejection = &Rejection{ReasonFileMismatch, ref.path + " differs from file.sha256 or file.size_bytes"}
			rec.Hash = ""
		}
	}
}

// safePath accepts clean relative slash paths without parent references.
func safePath(name string) bool {
	trimmed := strings.TrimSuffix(name, "/")
	return trimmed != "" && !strings.HasPrefix(trimmed, "/") && !strings.ContainsAny(trimmed, "\\\x00") &&
		path.Clean(trimmed) == trimmed && trimmed != ".." && !strings.HasPrefix(trimmed, "../")
}

type countingReader struct {
	r        io.Reader
	n, limit int64
	exceeded bool
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.limit {
		c.exceeded = true
		return n, errors.New("package is larger than the limit")
	}
	return n, err
}

// xmlGuard finds a document type or entity declaration in a file that starts as XML: a parser of a
// report could expand its entities or fetch an external one.
type xmlGuard struct {
	r                   io.Reader
	decided, xml, found bool
	tail                []byte
}

var _xmlDeclarations = [][]byte{[]byte("<!doctype"), []byte("<!entity")}

func (g *xmlGuard) Read(p []byte) (int, error) {
	n, err := g.r.Read(p)
	chunk := p[:n]
	if !g.decided {
		if trimmed := bytes.TrimLeft(chunk, " \t\r\n\ufeff"); len(trimmed) > 0 {
			g.decided, g.xml = true, trimmed[0] == '<'
		}
	}
	if g.xml && !g.found && n > 0 {
		window := bytes.ToLower(append(g.tail, chunk...))
		for _, d := range _xmlDeclarations {
			if bytes.Contains(window, d) {
				g.found = true
			}
		}
		g.tail = append(g.tail[:0], window[max(0, len(window)-len("<!doctype")):]...)
	}
	return n, err
}
