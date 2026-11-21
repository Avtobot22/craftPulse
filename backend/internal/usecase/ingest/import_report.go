package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	pathpkg "path"
	"regexp"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var (
	_commitSHA     = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	_absolutePath  = regexp.MustCompile(`^(/|[A-Za-z]:/)`)
	_reportFormats = map[string][]string{
		"findings": {"envelope", "sarif", "gitlab_security", "gitlab_code_quality"},
		"junit":    {"junit_xml"},
		"coverage": {"lcov", "cobertura", "jacoco"},
	}
	_sarifKinds = []string{
		entity.AnalysisCode, entity.AnalysisSAST, entity.AnalysisSCA, entity.AnalysisSecrets,
		entity.AnalysisIaC, entity.AnalysisImage, entity.AnalysisDocs, entity.AnalysisOpenAPI,
	}
)

func isReportImport(schema string) bool {
	return _reportFormats[schema] != nil
}

func projectReportImport(rc entity.Receipt, m *envelope.Manifest) bool {
	return rc.Channel == entity.ChannelImport && m.Area.Kind == envelope.AreaRefKindAnalysisProject && isReportImport(string(m.SchemaId))
}

func validateImportReport(f entity.ImportFile, v *entity.ValidationError) {
	switch {
	case f.CommitSHA == "":
		v.Add("/commit_sha", entity.FieldRequired, "")
	case !_commitSHA.MatchString(f.CommitSHA):
		v.Add("/commit_sha", entity.FieldInvalidFormat, "40 or 64 lowercase hexadecimal digits")
	}
	switch {
	case f.ReportFormat == "":
		v.Add("/report_format", entity.FieldRequired, "")
	case !slices.Contains(_reportFormats[f.SchemaID], f.ReportFormat):
		v.Add("/report_format", entity.FieldNotAllowedValue, "format does not match schema_id")
	}
	if f.ReportKind != "" && (f.ReportFormat != "sarif" || !slices.Contains(_sarifKinds, f.ReportKind)) {
		v.Add("/report_kind", entity.FieldNotAllowedValue, "expected a SARIF analysis kind")
	}
}

func validateImportPaths(f entity.ImportFile, v *entity.ValidationError) {
	if f.SourceRoot != "" {
		root := strings.ReplaceAll(f.SourceRoot, "\\", "/")
		switch {
		case f.SchemaID != "coverage":
			v.Add("/source_root", entity.FieldNotAllowedValue, "only coverage takes a source root")
		case len(root) > 4096 || strings.ContainsRune(root, 0) || !_absolutePath.MatchString(root):
			v.Add("/source_root", entity.FieldInvalidFormat, "expected an absolute checkout path")
		}
	}
	if f.SourceDir != "" {
		dir := strings.ReplaceAll(f.SourceDir, "\\", "/")
		clean := pathpkg.Clean(dir)
		switch {
		case f.SchemaID != "coverage":
			v.Add("/source_dir", entity.FieldNotAllowedValue, "only coverage takes a source directory")
		case len(dir) > 4096 || strings.ContainsRune(dir, 0) || _absolutePath.MatchString(dir) || clean == ".." || strings.HasPrefix(clean, "../"):
			v.Add("/source_dir", entity.FieldInvalidFormat, "expected a directory relative to the checkout root")
		}
	}
}

func addImportReport(w *envelope.Writer, m *envelope.Manifest, f entity.ImportFile) error {
	kind, media, extension, version := importReportMetadata(f)
	scope := sha256.Sum256([]byte(f.ProjectID.String() + "\x00" + kind + "\x00" + f.SourceRoot + "\x00" + f.SourceDir))
	scopeHash := hex.EncodeToString(scope[:])
	// The scope in the path distinguishes projects and mappings of the same file to repository paths.
	path := "files/reports/" + scopeHash + "/" + f.ReportFormat + extension
	ref, err := w.AddFile(path, media, f.Content)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(envelope.ReportFileV1{
		ReportKind: envelope.ReportFileV1ReportKind(kind), ReportFormat: envelope.ReportFileV1ReportFormat(f.ReportFormat),
		Sha: &f.CommitSHA, Producer: m.Producer.Name, ProducerVersion: m.Producer.Version,
	})
	if err != nil {
		return err
	}
	scan := &envelope.ScanManifestV1{
		SchemaVersion: envelope.ScanManifestV1SchemaVersionN1,
		Tools: []envelope.ScanToolRun{{
			Id: "import-" + f.ReportFormat, Producer: m.Producer.Name, ProducerVersion: m.Producer.Version,
			Mode: envelope.ScanToolRunModeImport, ReportKind: envelope.ScanToolRunReportKind(kind), Sha: &f.CommitSHA,
			ScopeHash: scopeHash, ExitClass: envelope.ScanToolRunExitClassUnknown,
			SourceRoot: importPath(f.SourceRoot), SourceDir: importPath(f.SourceDir),
			Completeness: envelope.ScanToolRunCompletenessUnknown, SkippedTargets: []envelope.ScanSkippedTarget{},
			Reports: []envelope.ScanReport{{
				File: ref, ReportKind: envelope.ScanReportReportKind(kind),
				ReportFormat: envelope.ScanReportReportFormat(f.ReportFormat), FormatVersion: version,
			}},
		}},
	}
	// The form has no tool timestamps. The zero times keep the package stable across repeat uploads.
	if err := w.AddScanManifest(scan); err != nil {
		return err
	}
	return w.AddRecord(&envelope.Record{
		RecordClass: envelope.RecordRecordClassFile, RecordKind: envelope.RecordRecordKindReportFile, PayloadSchema: "report_file/1",
		NativeNs: f.CommitSHA, NativeId: kind + ":" + path, SourceRevision: &ref.Sha256, File: &ref, Payload: payload,
	})
}

func importPath(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func importReportMetadata(f entity.ImportFile) (kind, media, ext, version string) {
	switch f.ReportFormat {
	case "sarif":
		kind := f.ReportKind
		if kind == "" {
			kind = entity.AnalysisCode
		}
		return kind, "application/sarif+json", ".sarif", "2.1.0"
	case "gitlab_security":
		var header struct {
			Scan struct {
				Type string `json:"type"`
			} `json:"scan"`
		}
		// The parser validates the file later and can report a malformed document in the receipt.
		_ = json.Unmarshal(f.Content, &header)
		kind := entity.AnalysisSAST
		switch header.Scan.Type {
		case "secret_detection":
			kind = entity.AnalysisSecrets
		case "dependency_scanning":
			kind = entity.AnalysisSCA
		}
		return kind, "application/json", ".json", "unknown"
	case "junit_xml":
		return entity.AnalysisJUnit, "application/xml", ".xml", "unknown"
	case "cobertura", "jacoco":
		return entity.AnalysisCoverage, "application/xml", ".xml", "unknown"
	case "lcov":
		return entity.AnalysisCoverage, "text/plain", ".info", "1"
	default:
		return entity.AnalysisCode, "application/json", ".json", "1"
	}
}
