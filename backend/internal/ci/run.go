package ci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _killGrace = 5 * time.Second

// Tool is one run of a tool of a profile image.
type Tool struct {
	ID              string
	ProfileID       string
	ComponentID     string
	Kind            string
	Format          string
	FormatVersion   string
	Producer        string
	ProducerVersion string
	Report          string
	Timeout         time.Duration
	FindingsCodes   []int
	ConfigFile      string
	RulesHash       string
	DBVersion       string
	ParserVersion   string
	Command         []string
	// Dir is the working directory of the command; empty means the directory of the step.
	Dir string
}

// Runner runs tools of the profile and records their runs in the job state.
type Runner struct {
	Job     *Job
	Profile *ciapi.CiCheckProfile
	Store   *Store
	Stdout  io.Writer
	Stderr  io.Writer
	Now     func() time.Time
}

// Run executes the tool within its timeout and records the run. A tool that runs out of time gives timeout
// and partial, a crash or a missing report gives failed; only a finished report is success or findings
// and complete (NFR-03). The tool outcome is a result, not an error of the step.
func (r *Runner) Run(ctx context.Context, t *Tool) (*envelope.ScanToolRun, error) {
	if len(t.Command) == 0 {
		return nil, errors.New("tool command is required")
	}
	if err := checkReport(t.Kind, t.Format); err != nil {
		return nil, err
	}
	entry, err := r.entry(t)
	if err != nil {
		return nil, err
	}
	run, err := r.toolRun(t, entry)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(t.Report); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove previous report: %w", err)
	}
	run.StartedAt = r.Now().UTC()
	code, timedOut, err := r.exec(ctx, t)
	run.FinishedAt = r.Now().UTC()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	finished := err == nil && reportReady(t.Report)
	if err := r.record(run, t, outcome(timedOut, finished, code, t.FindingsCodes)); err != nil {
		return nil, err
	}
	return run, nil
}

// record sets the outcome of the run, attaches the report of a finished run, complete or with skipped targets,
// and saves the run.
func (r *Runner) record(run *envelope.ScanToolRun, t *Tool, o toolOutcome) error {
	run.ExitClass, run.Completeness = o.exit, o.completeness
	var reports []string
	if o.finished() {
		reports = []string{t.Report}
		run.Reports = []envelope.ScanReport{{
			ReportKind: envelope.ScanReportReportKind(t.Kind), ReportFormat: envelope.ScanReportReportFormat(t.Format),
			FormatVersion: t.FormatVersion, File: envelope.FileRef{MediaType: mediaType(t.Format)},
		}}
	}
	return r.Store.AddRun(run, reports)
}

type toolOutcome struct {
	exit         envelope.ScanToolRunExitClass
	completeness envelope.ScanToolRunCompleteness
}

// finished tells whether the tool wrote its report: a run that ran out of time or failed has none.
func (o toolOutcome) finished() bool {
	switch o.exit {
	case envelope.ScanToolRunExitClassSuccess, envelope.ScanToolRunExitClassFindings, envelope.ScanToolRunExitClassPartial:
		return true
	case envelope.ScanToolRunExitClassFailed, envelope.ScanToolRunExitClassTimeout, envelope.ScanToolRunExitClassUnknown:
	}
	return false
}

// outcome classifies a tool run: out of time is timeout and partial, a crash or a missing report is
// failed; only a finished report is success or findings and complete (NFR-03).
func outcome(timedOut, finished bool, code int, findingsCodes []int) toolOutcome {
	switch {
	case timedOut:
		return toolOutcome{envelope.ScanToolRunExitClassTimeout, envelope.ScanToolRunCompletenessPartial}
	case finished && code == 0:
		return toolOutcome{envelope.ScanToolRunExitClassSuccess, envelope.ScanToolRunCompletenessComplete}
	case finished && slices.Contains(findingsCodes, code):
		return toolOutcome{envelope.ScanToolRunExitClassFindings, envelope.ScanToolRunCompletenessComplete}
	}
	return toolOutcome{envelope.ScanToolRunExitClassFailed, envelope.ScanToolRunCompletenessUnknown}
}

// entry finds the builtin row of the profile that the tool serves: its image digest and component.
func (r *Runner) entry(t *Tool) (Entry, error) {
	rows := Select(r.Profile, Filter{Kind: t.Kind, Mode: string(ciapi.CiCheckProfileEntriesModeBuiltin), ProfileID: t.ProfileID, ComponentID: t.ComponentID})
	switch {
	case len(rows) == 0:
		return Entry{}, fmt.Errorf("profile has no builtin %s check of profile %s", t.Kind, t.ProfileID)
	case len(rows) > 1:
		return Entry{}, fmt.Errorf("profile %s checks %s in %d components; pass the component", t.ProfileID, t.Kind, len(rows))
	case rows[0].ImageDigest == nil:
		return Entry{}, fmt.Errorf("profile row of %s has no image digest", t.ProfileID)
	}
	return rows[0], nil
}

func (r *Runner) toolRun(t *Tool, e Entry) (*envelope.ScanToolRun, error) {
	run := &envelope.ScanToolRun{
		Id: t.ID, Producer: t.Producer, ProducerVersion: t.ProducerVersion, Mode: envelope.ScanToolRunModeBuiltin,
		ProfileId: &e.ProfileID, ProfileImageDigest: e.ImageDigest, ReportKind: envelope.ScanToolRunReportKind(t.Kind),
		Component: &envelope.ScanComponent{Id: &e.ComponentID, PathPrefix: e.PathPrefix}, Language: optional(e.Language),
		RulesHash: optional(t.RulesHash), DbVersion: optional(t.DBVersion), ParserVersion: optional(t.ParserVersion), Sha: &r.Job.SHA, Ci: r.Job.ci(r.Job.JobID),
		SkippedTargets: []envelope.ScanSkippedTarget{}, Reports: []envelope.ScanReport{},
	}
	if t.ConfigFile != "" {
		data, err := os.ReadFile(t.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("read tool configuration: %w", err)
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		run.ConfigHash = &hash
	}
	run.ScopeHash = scopeHash(run)
	return run, nil
}

// exec runs the command; a command that cannot start is a failed tool, not an error of the step.
func (r *Runner) exec(ctx context.Context, t *Tool) (code int, timedOut bool, err error) {
	runCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, t.Command[0], t.Command[1:]...) //nolint:gosec // the CI configuration names the tool
	cmd.Stdout, cmd.Stderr, cmd.Env, cmd.Dir = r.Stdout, r.Stderr, toolEnv(), t.Dir
	cmd.WaitDelay = _killGrace
	ownGroup(cmd)
	err = cmd.Run()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return -1, true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), false, nil
	}
	if err != nil {
		fmt.Fprintf(r.Stderr, "craftpulse-ci: tool did not start: %v\n", err)
	}
	return 0, false, err
}

// scopeHash identifies what the run covers: component, language and kind of the check.
func scopeHash(run *envelope.ScanToolRun) string {
	scope := struct {
		Component string  `json:"component"`
		Language  *string `json:"language"`
		Kind      string  `json:"report_kind"`
	}{Language: run.Language, Kind: string(run.ReportKind)}
	if run.Component != nil {
		scope.Component = run.Component.PathPrefix
	}
	data, _ := json.Marshal(scope) // strings always encode
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// toolEnv is the environment of a tool without the upload token: no tool needs it.
func toolEnv(extra ...string) []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, TokenEnv+"=") })
	return append(env, extra...)
}

func checkReport(kind, format string) error {
	if !envelope.ScanReportReportKind(kind).Valid() {
		return fmt.Errorf("unknown report kind %q", kind)
	}
	if !envelope.ScanReportReportFormat(format).Valid() {
		return fmt.Errorf("unknown report format %q", format)
	}
	return nil
}

func reportReady(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func mediaType(format string) string {
	switch envelope.ScanReportReportFormat(format) {
	case envelope.ScanReportReportFormatSarif:
		return "application/sarif+json"
	case envelope.ScanReportReportFormatJunitXml, envelope.ScanReportReportFormatCobertura, envelope.ScanReportReportFormatJacoco:
		return "application/xml"
	case envelope.ScanReportReportFormatLcov:
		return "text/plain"
	case envelope.ScanReportReportFormatLizardCsv:
		return "text/csv"
	case envelope.ScanReportReportFormatEnvelope, envelope.ScanReportReportFormatGitlabSecurity, envelope.ScanReportReportFormatGitlabCodeQuality,
		envelope.ScanReportReportFormatGolangciJson, envelope.ScanReportReportFormatGitleaksJson, envelope.ScanReportReportFormatSpectralJson,
		envelope.ScanReportReportFormatDocsJson, envelope.ScanReportReportFormatJscpdJson, envelope.ScanReportReportFormatTrivyJson:
		return "application/json"
	}
	return "application/octet-stream"
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
