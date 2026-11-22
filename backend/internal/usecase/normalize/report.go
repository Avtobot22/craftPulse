package normalize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_factReport       = "report"
	_unknownOutcome   = "unknown"
	_reportFileRecord = "report_file"
)

// reportCandidate is a report file of a part with its provenance: from the scan manifest, or from the record alone
// for a file an adapter read from the CI of the source. Rejected holds the reason its record is rejected with; a
// file of a format without a parser yet has no parser and gives no report.
type reportCandidate struct {
	file      string
	run       *entity.SourceKey
	attempt   *entity.SourceKey
	component *envelope.ScanComponent
	parser    *reportParser
	paths     parseInput
	report    entity.ScanReport
	findings  []entity.FindingOccurrence
	tests     []parsedTest
	coverage  []entity.CoverageFact
	code      *entity.CodeFacts
	tool      *envelope.ScanToolRun
	rejected  string
}

func (in *input) reportFile(rec *envelope.Record, b baseFact, scan *envelope.ScanManifestV1) (*reportCandidate, error) {
	var p envelope.ReportFileV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode report file %s: %w", rec.NativeId, err)
	}
	if rec.File == nil {
		return nil, errorf("report file %s has no file", rec.NativeId)
	}
	c := &reportCandidate{
		file: rec.File.Path,
		report: entity.ScanReport{
			Record: b.ref, Kind: string(p.ReportKind), Mode: entity.CheckModeImport, Format: string(p.ReportFormat),
			SHA: deref(p.Sha), ArtifactDigest: deref(p.ArtifactDigest), StartedAt: p.StartedAt, FinishedAt: p.FinishedAt,
			ExitClass: _unknownOutcome, Completeness: _unknownOutcome,
		},
	}
	c.parser, _ = parserOf(c.report.Format)
	c.paths.kind = c.report.Kind
	c.paths.sha = c.report.SHA
	tool := scanTool(scan, rec.File.Path)
	if tool == nil {
		if p.ExitClass != nil {
			c.report.ExitClass = string(*p.ExitClass)
		}
		if p.Completeness != nil {
			c.report.Completeness = string(*p.Completeness)
		}
		job := ""
		if in.area.Kind == entity.ObjectCIProject {
			job = in.area.NativeID
		}
		c.run, c.attempt = in.ciObjects(job, p.RunNativeId, p.AttemptNativeId)
		return c, nil
	}
	return c, in.provenance(c, tool)
}

// provenance fills a report from its run of the scan manifest (DR-009).
func (in *input) provenance(c *reportCandidate, t *envelope.ScanToolRun) error {
	c.tool = t
	r := &c.report
	r.Mode, r.Producer, r.ProducerVersion = string(t.Mode), t.Producer, t.ProducerVersion
	r.ProfileID, r.DBVersion, r.DBPublishedAt = deref(t.ProfileId), deref(t.DbVersion), t.DbPublishedAt
	r.SHA, r.ArtifactDigest = deref(t.Sha), deref(t.ArtifactDigest)
	c.paths.sha = r.SHA
	started, finished := t.StartedAt, t.FinishedAt
	if !started.IsZero() {
		r.StartedAt = &started
	}
	if !finished.IsZero() {
		r.FinishedAt = &finished
	}
	r.ExitClass, r.Completeness = string(t.ExitClass), string(t.Completeness)
	r.ScannedCount, r.SkippedCount = t.ScannedCount, t.SkippedCount
	for _, s := range t.SkippedTargets {
		r.SkippedTargets = append(r.SkippedTargets, entity.SkippedTarget{Path: s.Path, Reason: string(s.Reason)})
	}
	if h := t.History; h != nil {
		r.History = &entity.ScanHistory{FromSHA: h.FromSha, ToSHA: h.ToSha, Truncated: h.Truncated}
	}
	var err error
	if r.RulesHash, err = hexHash(t.RulesHash); err != nil {
		return err
	}
	if r.ConfigHash, err = hexHash(t.ConfigHash); err != nil {
		return err
	}
	if r.ScopeHash, err = hexHash(&t.ScopeHash); err != nil {
		return err
	}
	c.component = t.Component
	c.paths.sourceRoot, c.paths.sourceDir = deref(t.SourceRoot), deref(t.SourceDir)
	r.Language = deref(t.Language)
	if t.Ci != nil {
		r.RefName = t.Ci.RefName
		c.run, c.attempt = in.ciObjects(deref(t.Ci.JobNativeId), &t.Ci.RunNativeId, t.Ci.AttemptNativeId)
	}
	return nil
}

// scanTool is the run of the scan manifest that wrote the file.
func scanTool(scan *envelope.ScanManifestV1, file string) *envelope.ScanToolRun {
	if scan == nil {
		return nil
	}
	for i := range scan.Tools {
		if slices.ContainsFunc(scan.Tools[i].Reports, func(r envelope.ScanReport) bool { return r.File.Path == file }) {
			return &scan.Tools[i]
		}
	}
	return nil
}

// ciObjects are the identities of the pipeline and the attempt of a report by the rules of CI runs; job names the
// job of a Jenkins build.
func (in *input) ciObjects(job string, run, attempt *string) (runKey, attemptKey *entity.SourceKey) {
	if id := deref(run); id != "" {
		k := in.runKey(job, id)
		runKey = &k
	}
	if id := deref(attempt); id != "" {
		k := in.attemptKey(deref(run), id)
		attemptKey = &k
	}
	return runKey, attemptKey
}

func hexHash(s *string) ([]byte, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(*s)
	if err != nil {
		return nil, errorf("decode hash of a scan: %w", err)
	}
	return b, nil
}

// readReports parses the report files of a part before the transaction: the stored package is read once more as a
// stream, file by file.
func (u *UseCase) readReports(ctx context.Context, rc entity.Receipt, in *input) error {
	byFile := map[string][]*reportCandidate{}
	manifests := map[string][]*manifestCandidate{}
	var names []string
	for _, c := range in.reports {
		if c.parser == nil {
			continue
		}
		if _, ok := byFile[c.file]; !ok {
			names = append(names, c.file)
		}
		byFile[c.file] = append(byFile[c.file], c)
	}
	for _, c := range in.manifests {
		if c.file == "" || c.parser == nil || c.rejected != "" {
			continue
		}
		if _, ok := byFile[c.file]; !ok && manifests[c.file] == nil {
			names = append(names, c.file)
		}
		manifests[c.file] = append(manifests[c.file], c)
	}
	if len(names) == 0 {
		return nil
	}
	body, err := u.d.Objects.Get(ctx, rc.ObjectKey)
	if err != nil {
		return err
	}
	defer body.Close()
	return envelope.Files(body, names, func(name string, content io.Reader) error {
		if list := manifests[name]; len(list) > 0 {
			return readManifestFile(in, list, content)
		}
		first := byFile[name][0]
		src := &sourceReader{r: content}
		parsed, err := first.parser.read(src, first.paths)
		if src.err != nil {
			return src.err
		}
		var rejected *errReport
		if err != nil && !errors.As(err, &rejected) {
			return err
		}
		var coverage []entity.CoverageFact
		if rejected == nil {
			if coverage, err = u.lineMaps(ctx, rc, name, parsed.coverage); err != nil {
				return err
			}
		}
		for _, c := range byFile[name] {
			if rejected != nil {
				c.rejected = rejected.reason
				continue
			}
			c.findings, c.tests, c.coverage = parsed.findings, parsed.tests, coverage
			c.code = parsed.scopedCodeFacts(c.tool)
			c.report.DependencyDelta = parsed.dependencyDelta
			c.report.SkippedTargets = append(c.report.SkippedTargets, parsed.excluded...)
			c.report.SchemaVersion, c.report.ParserVersion = parsed.formatVersion, first.parser.version
		}
		return nil
	})
}

// lineMaps puts the map of lines of a coverage report in the object store before the transaction refers to it: one
// object per file of the package, a line per file and kind, so normalizing the package again writes the same object.
func (u *UseCase) lineMaps(ctx context.Context, rc entity.Receipt, file string, coverage []parsedCoverage) ([]entity.CoverageFact, error) {
	if len(coverage) == 0 {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(file))
	key := "line-maps/" + rc.OrganizationID.String() + "/" + rc.ID.String() + "/" + hex.EncodeToString(sum[:]) + ".jsonl"
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	facts := make([]entity.CoverageFact, len(coverage))
	for i := range coverage {
		c := &coverage[i]
		line := struct {
			Path   string    `json:"path"`
			Kind   string    `json:"kind"`
			Points [][]int64 `json:"points"`
		}{c.path, c.kind, c.points}
		if line.Points == nil {
			line.Points = [][]int64{}
		}
		if err := enc.Encode(line); err != nil {
			return nil, errorf("encode line map of %s: %w", c.path, err)
		}
		facts[i] = entity.CoverageFact{Path: c.path, Kind: c.kind, Covered: c.covered, Executable: c.executable, LineMapKey: key}
	}
	if err := u.d.Objects.Put(ctx, key, &buf); err != nil {
		return nil, errorf("put line map: %w", err)
	}
	return facts, nil
}

func readManifestFile(in *input, list []*manifestCandidate, content io.Reader) error {
	src := &sourceReader{r: content}
	parsed, err := list[0].parser.read(src, parseInput{kind: _recordExecutionManifest})
	if src.err != nil {
		return src.err
	}
	reason := ""
	if rejected := (*errReport)(nil); errors.As(err, &rejected) {
		reason = rejected.reason
	} else if err != nil {
		return err
	}
	for _, c := range list {
		if err := in.readManifest(c, parsed, reason); err != nil {
			return err
		}
	}
	return nil
}

// sourceReader keeps an error of reading the stored package apart from a broken report.
type sourceReader struct {
	r   io.Reader
	err error
}

func (s *sourceReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		s.err = err
	}
	return n, err
}

// reports writes the reports of a part: a record that already gave a report adds nothing, and a file normalize could
// not read rejects its record, so the report accepted before for the same area, SHA and kind stays in force.
func (w *writer) reports(ctx context.Context, in *input) error {
	if len(in.reports) == 0 {
		return nil
	}
	reasons := map[int]string{}
	var (
		valid    []*reportCandidate
		rejected []*reportCandidate
		nos      []int
	)
	for _, c := range in.reports {
		switch {
		case c.rejected != "":
			reasons[c.report.Record.RecordNo] = c.rejected
			rejected = append(rejected, c)
		case c.parser != nil:
			valid = append(valid, c)
			nos = append(nos, c.report.Record.RecordNo)
		}
	}
	if len(reasons) > 0 {
		if err := w.u.d.Ingest.RejectRecords(ctx, w.org, in.receipt, reasons); err != nil {
			return err
		}
		if err := w.rejectedReports(ctx, in, rejected); err != nil {
			return err
		}
	}
	if len(valid) == 0 {
		return nil
	}
	done, err := w.u.d.Repo.ReportedRecords(ctx, w.org, in.receipt, nos)
	if err != nil {
		return err
	}
	area := w.objects[in.area].ID
	components, err := w.components(ctx, valid, area)
	if err != nil {
		return err
	}
	attempts, err := w.testAttempts(ctx, valid)
	if err != nil {
		return err
	}
	refs, err := w.runRefs(ctx, valid)
	if err != nil {
		return err
	}
	for _, c := range valid {
		if slices.Contains(done, c.report.Record.RecordNo) {
			continue
		}
		r := c.report
		r.KnownSeq, r.AreaID = w.seq, area
		r.ComponentID = componentOf(components, c.component)
		if c.run != nil {
			id := w.objects[*c.run].ID
			r.RunID = &id
			if r.RefName == "" {
				r.RefName = refs[id]
			}
		}
		if c.attempt != nil {
			id := w.objects[*c.attempt].ID
			r.AttemptID = &id
		}
		rows := entity.ReportRows{Findings: c.findings, Tests: w.executions(c, r.AttemptID, attempts), Coverage: c.coverage, Code: c.code}
		if err := w.u.d.Repo.InsertReport(ctx, w.org, &r, rows); err != nil {
			return err
		}
		w.changes.add(area, _factReport, changedAt(r.StartedAt, r.FinishedAt))
	}
	return nil
}

func (w *writer) rejectedReports(ctx context.Context, in *input, reports []*reportCandidate) error {
	area := w.objects[in.area].ID
	components, err := w.components(ctx, reports, area)
	if err != nil {
		return err
	}
	for _, c := range reports {
		r := c.report
		r.KnownSeq, r.AreaID = w.seq, area
		r.ComponentID = componentOf(components, c.component)
		if c.run != nil {
			id := w.objects[*c.run].ID
			r.RunID = &id
		}
		if c.attempt != nil {
			id := w.objects[*c.attempt].ID
			r.AttemptID = &id
		}
		if err := w.u.d.Repo.InsertRejectedReport(ctx, w.org, r, c.rejected); err != nil {
			return err
		}
		w.changes.add(area, _factReport, changedAt(r.StartedAt, r.FinishedAt))
	}
	return nil
}

// runRefs are the branches of the runs of reports that came without a scan manifest, read after the CI facts of
// the part are written.
func (w *writer) runRefs(ctx context.Context, reports []*reportCandidate) (map[entity.ID]string, error) {
	var ids []entity.ID
	for _, c := range reports {
		if c.report.RefName == "" && c.run != nil {
			ids = append(ids, w.objects[*c.run].ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := w.u.d.Repo.CurrentCIRunVersions(ctx, w.org, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[entity.ID]string, len(list))
	for i := range list {
		out[list[i].RunID] = list[i].RefName
	}
	return out, nil
}

// testAttempts are the current versions of the attempts JUnit reports of the part belong to, read after the CI facts
// of the part are written.
func (w *writer) testAttempts(ctx context.Context, reports []*reportCandidate) (map[entity.ID]entity.CIAttemptVersion, error) {
	var ids []entity.ID
	for _, c := range reports {
		if len(c.tests) > 0 && c.attempt != nil {
			ids = append(ids, w.objects[*c.attempt].ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := w.u.d.Repo.CurrentCIAttemptVersions(ctx, w.org, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[entity.ID]entity.CIAttemptVersion, len(list))
	for i := range list {
		out[list[i].AttemptID] = list[i]
	}
	return out, nil
}

// executions are the test cases of a report in the configuration of its attempt (DR-006). An attempt not collected
// yet stands for its own configuration, so the jobs of a matrix stay apart; a report without an attempt has an
// unknown one. The run starts with the attempt, or else with the report.
func (w *writer) executions(c *reportCandidate, attempt *entity.ID, attempts map[entity.ID]entity.CIAttemptVersion) []entity.TestExecution {
	if len(c.tests) == 0 {
		return nil
	}
	config := fingerprint("config", "unknown")
	started := w.now
	if c.report.FinishedAt != nil {
		started = *c.report.FinishedAt
	}
	if c.report.StartedAt != nil {
		started = *c.report.StartedAt
	}
	if attempt != nil {
		config = fingerprint("config", "attempt", c.attempt.NativeID)
		if v, ok := attempts[*attempt]; ok {
			config = v.ConfigFingerprint
			if v.StartedAt != nil {
				started = *v.StartedAt
			}
		}
	}
	out := make([]entity.TestExecution, len(c.tests))
	for i := range c.tests {
		t := &c.tests[i]
		out[i] = entity.TestExecution{
			CaseKey: t.key, CaseKeyHash: t.keyHash, AttemptID: attempt, ConfigFingerprint: config, Ordinal: t.ordinal,
			Outcome: t.outcome, DurationMS: t.durationMS, RunStartedAt: started, Suite: &t.suite,
		}
	}
	return out
}

// components are the confirmed components of the repository, read once when a report names one.
func (w *writer) components(ctx context.Context, reports []*reportCandidate, area entity.ID) ([]entity.Component, error) {
	if !slices.ContainsFunc(reports, func(c *reportCandidate) bool { return c.component != nil }) {
		return nil, nil
	}
	return w.u.d.Accounts.RepositoryComponents(ctx, w.org, area)
}

// componentOf finds the component of a report by the path prefix of the scan manifest; of several components of
// one path in different services the one the manifest names wins, otherwise the report has none.
func componentOf(components []entity.Component, c *envelope.ScanComponent) *entity.ID {
	if c == nil {
		return nil
	}
	prefix := strings.TrimSuffix(c.PathPrefix, "/")
	var found []entity.ID
	for i := range components {
		if strings.TrimSuffix(components[i].PathPrefix, "/") == prefix {
			found = append(found, components[i].ID)
		}
	}
	if c.Id != nil {
		if i := slices.Index(found, entity.ID(*c.Id)); i >= 0 {
			return &found[i]
		}
	}
	if len(found) == 1 {
		return &found[0]
	}
	return nil
}
