package envelope

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	ScanManifestName                 = "scan-manifest.json"
	ExecutionManifestName            = "execution-manifest.json"
	ReasonMissingScanManifest        = "missing_scan_manifest"
	ReasonInvalidScanManifest        = "invalid_scan_manifest"
	ReasonInvalidExecutionManifest   = "invalid_execution_manifest"
	ReasonUnsupportedManifestVersion = "unsupported_manifest_version"
)

// ValidateScanManifest checks the version, schema and provenance conditions before any report is accepted.
func ValidateScanManifest(data []byte) (*ScanManifestV1, *Rejection) {
	rejection := validateCIManifest(data, _rules.ScanManifest, ReasonInvalidScanManifest)
	if rejection != nil {
		return nil, rejection
	}
	var m ScanManifestV1
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, &Rejection{ReasonInvalidScanManifest, "manifest does not decode"}
	}
	if problem := scanSemantics(&m); problem != "" {
		return nil, &Rejection{ReasonInvalidScanManifest, problem}
	}
	return &m, nil
}

// ValidateExecutionManifest accepts the unchanged v1 format of the Jenkins demo builds.
func ValidateExecutionManifest(data []byte) (*JenkinsExecutionManifestV1, *Rejection) {
	rejection := validateCIManifest(data, _rules.ExecutionManifest, ReasonInvalidExecutionManifest)
	if rejection != nil {
		return nil, rejection
	}
	var m JenkinsExecutionManifestV1
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, &Rejection{ReasonInvalidExecutionManifest, "manifest does not decode"}
	}
	if problem := executionSemantics(&m); problem != "" {
		return nil, &Rejection{ReasonInvalidExecutionManifest, problem}
	}
	return &m, nil
}

func validateCIManifest(data []byte, s *schema, code string) *Rejection {
	value, err := decode(data)
	m, ok := value.(map[string]any)
	if err != nil || !ok {
		return &Rejection{code, "manifest must be a JSON object"}
	}
	if version, ok := m["schema_version"].(string); ok && version != "1" {
		return &Rejection{ReasonUnsupportedManifestVersion, "schema_version is not supported"}
	}
	if bad := s.validate(m, ""); bad != nil {
		return &Rejection{code, bad.Error()}
	}
	return nil
}

func scanSemantics(m *ScanManifestV1) string {
	ids := map[string]bool{}
	paths := map[string]bool{}
	for i := range m.Tools {
		r := &m.Tools[i]
		at := "tools[" + strconv.Itoa(i) + "]"
		switch {
		case ids[r.Id]:
			return at + ".id: duplicate tool run"
		case r.Sha == nil && r.ArtifactDigest == nil:
			return at + ".sha: sha or artifact_digest is required"
		case r.FinishedAt.Before(r.StartedAt):
			return at + ".finished_at: precedes started_at"
		case r.Mode == ScanToolRunModeBuiltin && (r.ProfileId == nil || r.ProfileImageDigest == nil):
			return at + ".profile_id: builtin requires a profile and image digest"
		case r.SkippedCount != nil && *r.SkippedCount != len(r.SkippedTargets):
			return at + ".skipped_count: differs from skipped_targets"
		case r.EligibleFileCount != nil && r.ScannedCount != nil && *r.ScannedCount > *r.EligibleFileCount:
			return at + ".scanned_count: exceeds eligible_file_count"
		case r.Completeness == ScanToolRunCompletenessComplete && (len(r.SkippedTargets) > 0 || (r.History != nil && r.History.Truncated)):
			return at + ".completeness: skipped or truncated scope cannot be complete"
		case r.DbPublishedAt != nil && r.DbVersion == nil:
			return at + ".db_version: required with db_published_at"
		}
		ids[r.Id] = true
		if r.Component != nil && !sourcePath(r.Component.PathPrefix) {
			return at + ".component.path_prefix: unsafe path"
		}
		for _, target := range r.SkippedTargets {
			if !sourcePath(target.Path) {
				return at + ".skipped_targets: unsafe path"
			}
		}
		if r.ExcludedTargets != nil {
			for _, target := range *r.ExcludedTargets {
				if !safePath(target.Path) {
					return at + ".excluded_targets: unsafe path"
				}
			}
		}
		if r.TestTargets != nil {
			for _, target := range *r.TestTargets {
				if !safePath(target) {
					return at + ".test_targets: unsafe path"
				}
			}
		}
		for j := range r.Reports {
			report := &r.Reports[j]
			if !strings.HasPrefix(report.File.Path, FilesDir) || !safePath(report.File.Path) || paths[report.File.Path] {
				return at + ".reports: unsafe or duplicate file path"
			}
			if string(report.ReportKind) != string(r.ReportKind) {
				return at + ".reports: report_kind differs from tool run"
			}
			paths[report.File.Path] = true
		}
	}
	return ""
}

func sourcePath(p string) bool { return p == "" || safePath(p) }

func executionSemantics(m *JenkinsExecutionManifestV1) string {
	address, err := url.Parse(m.BuildUrl)
	if err != nil || address.Scheme != "https" || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return "build_url: expected HTTPS URL without credentials, query or fragment"
	}
	attempts := map[string]bool{}
	datasetIDs := map[string]bool{}
	for i := range m.Steps {
		s := &m.Steps[i]
		if s.FinishedAt.Before(s.StartedAt) {
			return "steps.finished_at: precedes started_at"
		}
		hash, err := configurationHash(s.Axes)
		if err != nil || hash != s.ConfigurationHash {
			return "steps.configuration_hash: differs from matrix axes"
		}
		identity := JenkinsAttemptKey(m.Job, m.BuildNumber, s.Stage, s.Name, s.ConfigurationHash, s.Attempt)
		if attempts[identity] || datasetIDs[s.DatasetAttemptId] {
			return "steps: duplicate attempt identity"
		}
		attempts[identity] = true
		datasetIDs[s.DatasetAttemptId] = true
		for _, r := range s.Reports {
			if !safePath(r.Path) {
				return "steps.reports.path: unsafe relative path"
			}
		}
	}
	return ""
}

// JenkinsAttemptKey is local to the source instance; matrix axes and the internal retry number distinguish attempts.
func JenkinsAttemptKey(job string, build int, stage, name, config string, attempt int) string {
	encoded, _ := json.Marshal([]any{job, build, stage, name, config, attempt})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (c *checker) readCIManifest(r io.Reader, h *tar.Header) *Rejection {
	if h.Size > _maxManifestBytes {
		return &Rejection{ReasonLimitExceeded, h.Name + " is larger than 1 MiB"}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return &Rejection{ReasonInvalidArchive, "cannot read " + h.Name}
	}
	if h.Name == ScanManifestName {
		m, rejection := ValidateScanManifest(data)
		if rejection != nil {
			return rejection
		}
		c.result.ScanManifest = m
		c.scanRaw = data
	} else {
		m, rejection := ValidateExecutionManifest(data)
		if rejection != nil {
			return rejection
		}
		c.result.ExecutionManifest = m
		c.executionRaw = data
	}
	return nil
}

func (c *checker) checkCIManifests() *Rejection {
	if c.result.ScanManifest == nil && c.result.ExecutionManifest == nil && c.page {
		return nil
	}
	if (c.reportDeclared || (len(c.files) > 0 && slices.Contains(c.rule.Kinds, "report_file"))) && c.result.ScanManifest == nil {
		return &Rejection{ReasonMissingScanManifest, "report files require " + ScanManifestName}
	}
	type origin struct {
		tool   *ScanToolRun
		report *ScanReport
	}
	reports := map[string]origin{}
	if m := c.result.ScanManifest; m != nil {
		for i := range m.Tools {
			for j := range m.Tools[i].Reports {
				r := &m.Tools[i].Reports[j]
				actual, ok := c.files[r.File.Path]
				if !ok || actual.sha256 != r.File.Sha256 || actual.size != int64(r.File.SizeBytes) {
					return &Rejection{ReasonInvalidScanManifest, "reports.file: missing file or mismatched SHA-256/size"}
				}
				reports[r.File.Path] = origin{&m.Tools[i], r}
			}
		}
	}
	for _, ref := range c.fileRefs {
		rec := &c.result.Records[ref.result]
		if rec.Kind != "report_file" || rec.Rejection != nil {
			continue
		}
		source, ok := reports[ref.path]
		if !ok {
			return &Rejection{ReasonInvalidScanManifest, "report_file.file: absent from scan manifest"}
		}
		p := ref.payload
		t, r := source.tool, source.report
		if !reportProvenanceMatches(p, t, r) {
			return &Rejection{ReasonInvalidScanManifest, "report_file.payload: provenance differs from scan manifest"}
		}
	}

	if m := c.result.ExecutionManifest; m != nil {
		for i := range m.Steps {
			for _, r := range m.Steps[i].Reports {
				p := r.Path
				if !strings.HasPrefix(p, FilesDir) {
					p = FilesDir + p
				}
				actual, ok := c.files[p]
				if !ok || actual.sha256 != r.Sha256 {
					return &Rejection{ReasonInvalidExecutionManifest, "steps.reports: missing file or mismatched SHA-256"}
				}
				source, ok := reports[p]
				if !ok {
					return &Rejection{ReasonInvalidExecutionManifest, "steps.reports: absent from scan manifest"}
				}
				ci := source.tool.Ci
				step := &m.Steps[i]
				identity := JenkinsAttemptKey(m.Job, m.BuildNumber, step.Stage, step.Name, step.ConfigurationHash, step.Attempt)
				format := string(r.Format)
				if r.Format == JenkinsExecutionManifestV1StepsReportsFormatJunit {
					format = string(ScanReportReportFormatJunitXml)
				}
				if ci == nil || ci.Provider != ScanCiCoordinatesProviderJenkins || valueOf(ci.JobNativeId) != m.Job || ci.RunNativeId != strconv.Itoa(m.BuildNumber) || valueOf(ci.AttemptNativeId) != identity || ci.AttemptNo == nil || *ci.AttemptNo != step.Attempt || string(source.report.ReportFormat) != format {
					return &Rejection{ReasonInvalidExecutionManifest, "steps.reports: attempt or format differs from scan manifest"}
				}
				if source.tool.Sha != nil && *source.tool.Sha != r.SourceSha {
					return &Rejection{ReasonInvalidExecutionManifest, "steps.reports.source_sha: differs from scan manifest"}
				}
			}
		}
	}
	return nil
}

// Optional legacy record fields may be absent, but a supplied value must agree with the scan.
func reportProvenanceMatches(p map[string]any, t *ScanToolRun, r *ScanReport) bool {
	for field, expected := range map[string]string{
		"report_kind": string(r.ReportKind), "report_format": string(r.ReportFormat),
		"producer": t.Producer, "producer_version": t.ProducerVersion,
		"sha": valueOf(t.Sha), "artifact_digest": valueOf(t.ArtifactDigest),
	} {
		if text(p, field) != expected {
			return false
		}
	}
	run, attempt := "", ""
	if t.Ci != nil {
		run, attempt = t.Ci.RunNativeId, valueOf(t.Ci.AttemptNativeId)
	}
	for field, expected := range map[string]string{
		"profile_id": valueOf(t.ProfileId), "rules_hash": valueOf(t.RulesHash),
		"config_hash": valueOf(t.ConfigHash), "db_version": valueOf(t.DbVersion),
		"exit_class": string(t.ExitClass), "completeness": string(t.Completeness),
		"run_native_id": run, "attempt_native_id": attempt,
	} {
		if p[field] != nil && text(p, field) != expected {
			return false
		}
	}
	for field, expected := range map[string]time.Time{"started_at": t.StartedAt, "finished_at": t.FinishedAt} {
		if p[field] == nil {
			continue
		}
		actual, err := time.Parse(time.RFC3339Nano, text(p, field))
		if err != nil || !actual.Equal(expected) {
			return false
		}
	}
	return true
}

func (w *Writer) AddScanManifest(m *ScanManifestV1) error {
	return w.addCIManifest(ScanManifestName, m)
}

func (w *Writer) AddExecutionManifest(m *JenkinsExecutionManifestV1) error {
	return w.addCIManifest(ExecutionManifestName, m)
}

func (w *Writer) addCIManifest(name string, m any) error {
	if w.closed {
		return ErrWriterClosed
	}
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	return w.entry(name, data)
}

func configurationHash(axes map[string]string) (string, error) {
	data, err := json.Marshal(axes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
