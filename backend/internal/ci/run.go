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
	Command         []string
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
	var reports []string
	finished := err == nil && reportReady(t.Report)
	switch {
	case timedOut:
		run.ExitClass, run.Completeness = envelope.ScanToolRunExitClassTimeout, envelope.ScanToolRunCompletenessPartial
	case finished && code == 0:
		run.ExitClass, run.Completeness = envelope.ScanToolRunExitClassSuccess, envelope.ScanToolRunCompletenessComplete
	case finished && slices.Contains(t.FindingsCodes, code):
		run.ExitClass, run.Completeness = envelope.ScanToolRunExitClassFindings, envelope.ScanToolRunCompletenessComplete
	default:
		run.ExitClass, run.Completeness = envelope.ScanToolRunExitClassFailed, envelope.ScanToolRunCompletenessUnknown
	}
	if run.Completeness == envelope.ScanToolRunCompletenessComplete {
		reports = []string{t.Report}
		run.Reports = []envelope.ScanReport{{
			ReportKind: envelope.ScanReportReportKind(t.Kind), ReportFormat: envelope.ScanReportReportFormat(t.Format),
			FormatVersion: t.FormatVersion, File: envelope.FileRef{MediaType: mediaType(t.Format)},
		}}
	}
	if err := r.Store.AddRun(run, reports); err != nil {
		return nil, err
	}
	return run, nil
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
		RulesHash: optional(t.RulesHash), DbVersion: optional(t.DBVersion), Sha: &r.Job.SHA, Ci: r.Job.ci(r.Job.JobID),
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
	cmd.Stdout, cmd.Stderr, cmd.Env = r.Stdout, r.Stderr, toolEnv()
	cmd.WaitDelay = _killGrace
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
	case envelope.ScanReportReportFormatEnvelope, envelope.ScanReportReportFormatGitlabSecurity, envelope.ScanReportReportFormatGitlabCodeQuality,
		envelope.ScanReportReportFormatGolangciJson, envelope.ScanReportReportFormatGitleaksJson, envelope.ScanReportReportFormatSpectralJson,
		envelope.ScanReportReportFormatDocsJson:
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
