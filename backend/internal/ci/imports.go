package ci

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// ReportSpec names a client report as kind:format:path in the CI configuration.
type ReportSpec struct {
	Kind, Format, Path string
}

func ParseReportSpec(s string) (ReportSpec, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[2] == "" {
		return ReportSpec{}, fmt.Errorf("report %q: expected kind:format:path", s)
	}
	spec := ReportSpec{Kind: parts[0], Format: parts[1], Path: parts[2]}
	if err := checkReport(spec.Kind, spec.Format); err != nil {
		return ReportSpec{}, fmt.Errorf("report %q: %w", s, err)
	}
	return spec, nil
}

// Import is a set of client reports produced by one tool of the client pipeline.
type Import struct {
	Reports         []ReportSpec
	Producer        string
	ProducerVersion string
	FormatVersion   string
	// ProducerJob is the ID of the job that produced the reports, when the step knows it: reports from the
	// artifacts of another job carry only the pipeline and the SHA.
	ProducerJob string
	// SourceRoot is the checkout the tool ran in and SourceDir the directory of the repository its relative
	// paths start from; normalize turns the paths of a report into paths of the repository by them.
	SourceRoot, SourceDir string
}

// Importer records client reports as import runs in the job state.
type Importer struct {
	Job     *Job
	Profile *ciapi.CiCheckProfile
	Store   *Store
	Now     func() time.Time
}

// Add records one run per report. A missing or empty file is recorded as a failed run without a report:
// the absence of a report is not a clean result.
func (im *Importer) Add(in *Import) ([]envelope.ScanToolRun, error) {
	if in.ProducerJob != "" && !_nativeID.MatchString(in.ProducerJob) {
		return nil, errors.New("producer job must be a GitLab job ID")
	}
	runs := make([]envelope.ScanToolRun, 0, len(in.Reports))
	for i, spec := range in.Reports {
		run := envelope.ScanToolRun{
			Id: "import-" + spec.Kind + "-" + strconv.Itoa(i+1), Producer: in.Producer, ProducerVersion: in.ProducerVersion,
			Mode: envelope.ScanToolRunModeImport, ReportKind: envelope.ScanToolRunReportKind(spec.Kind), Sha: &im.Job.SHA,
			Ci: im.Job.ci(in.ProducerJob), SkippedTargets: []envelope.ScanSkippedTarget{}, Reports: []envelope.ScanReport{},
			ExitClass: envelope.ScanToolRunExitClassFailed, Completeness: envelope.ScanToolRunCompletenessUnknown,
			StartedAt: im.Now().UTC(), FinishedAt: im.Now().UTC(), SourceRoot: optional(in.SourceRoot), SourceDir: optional(in.SourceDir),
		}
		if rows := Select(im.Profile, Filter{Kind: spec.Kind, Mode: string(ciapi.CiCheckProfileEntriesModeImport)}); len(rows) == 1 {
			run.ProfileId = &rows[0].ProfileID
			run.Component = &envelope.ScanComponent{Id: &rows[0].ComponentID, PathPrefix: rows[0].PathPrefix}
			run.Language = optional(rows[0].Language)
		}
		run.ScopeHash = scopeHash(&run)
		var files []string
		info, err := os.Stat(spec.Path)
		if err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			// The client tool finished before the step; the file time is the best known time of the run.
			run.StartedAt, run.FinishedAt = info.ModTime().UTC(), info.ModTime().UTC()
			run.ExitClass, run.Completeness = envelope.ScanToolRunExitClassSuccess, envelope.ScanToolRunCompletenessComplete
			run.Reports = []envelope.ScanReport{{
				ReportKind: envelope.ScanReportReportKind(spec.Kind), ReportFormat: envelope.ScanReportReportFormat(spec.Format),
				FormatVersion: in.FormatVersion, File: envelope.FileRef{MediaType: mediaType(spec.Format)},
			}}
			files = []string{spec.Path}
		}
		if err := im.Store.AddRun(&run, files); err != nil {
			return nil, fmt.Errorf("report %s: %w", spec.Path, err)
		}
		runs = append(runs, run)
	}
	return runs, nil
}
