// Command craftpulse-ci is the craftPulse step in the client pipeline: it reads the check profile of the
// project, runs profile tools or collects ready reports, packs them and uploads the package (TZ 10.5.2).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

// Exit codes. Findings in an accepted package are not a failure of the step.
const (
	_exitFailed  = 1
	_exitRefused = 2
	_exitUnknown = 3
)

const (
	_defaultWait    = 2 * time.Minute
	_defaultTimeout = 15 * time.Minute
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := execute(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv, ci.ClientConfig{})
	stop()
	os.Exit(code)
}

// app carries what every command shares: output, environment and the replaceable network of tests.
type app struct {
	stdout, stderr io.Writer
	getenv         func(string) string
	client         ci.ClientConfig
	debug          bool
	workDir        string
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string, cfg ci.ClientConfig) int {
	secrets := []string{getenv(ci.TokenEnv)}
	a := &app{stdout: logger.Redact(stdout, secrets), stderr: logger.Redact(stderr, secrets), getenv: getenv, client: cfg}
	root := a.root() //nolint:contextcheck // commands take the context from cmd.Context()
	root.SetArgs(args)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	fmt.Fprintln(a.stderr, "craftpulse-ci: "+err.Error())
	var refused *ci.RefusedError
	switch {
	case errors.As(err, &refused):
		return _exitRefused
	case errors.Is(err, ci.ErrOutcomeUnknown):
		return _exitUnknown
	}
	return _exitFailed
}

func (a *app) root() *cobra.Command {
	root := &cobra.Command{
		Use:   "craftpulse-ci",
		Short: "craftPulse step of a GitLab CI pipeline",
		Long: `craftpulse-ci reads the check profile of the project, records tool runs and client reports of the
job and uploads them to craftPulse as one package.

Settings: CRAFTPULSE_URL (site address), CRAFTPULSE_UPLOAD_TOKEN (upload token of the project, sent
only as a Bearer header and never printed), CRAFTPULSE_WORK_DIR (state of the job, a temporary
directory by default), CRAFTPULSE_INSTANCE_KEY (GitLab instance key, CI_SERVER_HOST by default),
CRAFTPULSE_DEBUG=1 (print requests and answers). CI coordinates come from the predefined GitLab CI
variables.

Exit codes: 0 - the package is accepted, findings included; 1 - the step failed; 2 - the intake
refused the package or rejected it whole; 3 - the intake result did not arrive in time.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			a.debug = a.debug || a.getenv("CRAFTPULSE_DEBUG") == "1"
		},
	}
	root.PersistentFlags().BoolVar(&a.debug, "debug", false, "print every request and answer, without the token")
	root.PersistentFlags().StringVar(&a.workDir, "work-dir", "", "directory with the state of the job")
	root.AddCommand(a.versionCmd(), a.profileCmd(), a.runCmd(), a.importCmd(), a.uploadCmd(), a.fixturesCmd())
	return root
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "print the version", Args: cobra.NoArgs,
		Run: func(*cobra.Command, []string) { fmt.Fprintln(a.stdout, "craftpulse-ci "+health.CommitSHA()) },
	}
}

// session is the job, its state and a client of the API; the profile is read once per job and kept.
type session struct {
	job     ci.Job
	store   *ci.Store
	client  *ci.Client
	profile *ciapi.CiCheckProfile
}

func (a *app) session(ctx context.Context, fresh bool) (*session, error) {
	job, err := ci.JobFromEnv(a.getenv)
	if err != nil {
		return nil, err
	}
	if a.workDir != "" {
		job.WorkDir = a.workDir
	}
	cfg := a.client
	cfg.Out = a.stdout
	if a.debug {
		base := http.DefaultTransport
		if cfg.HTTP != nil && cfg.HTTP.Transport != nil {
			base = cfg.HTTP.Transport
		}
		httpClient := &http.Client{Timeout: 2 * time.Minute, Transport: ci.DebugTransport(base, a.stderr)}
		cfg.HTTP = httpClient
	}
	client, err := ci.NewClient(&job, cfg)
	if err != nil {
		return nil, err
	}
	s := &session{job: job, store: ci.NewStore(job.WorkDir), client: client}
	if !fresh {
		var ok bool
		if s.profile, ok, err = s.store.Profile(); err != nil || ok {
			return s, err
		}
	}
	if s.profile, err = client.Profile(ctx); err != nil {
		return nil, err
	}
	return s, s.store.SaveProfile(s.profile)
}

func (a *app) profileCmd() *cobra.Command {
	var f ci.Filter
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "read the check profile of the project and print its rows for this job",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := a.session(cmd.Context(), true)
			if err != nil {
				return err
			}
			ci.PrintJob(a.stdout, health.CommitSHA(), &s.job, s.profile)
			ci.PrintEntries(a.stdout, ci.Select(s.profile, f))
			return nil
		},
	}
	cmd.Flags().StringVar(&f.Kind, "kind", "", "analysis kind of the job")
	cmd.Flags().StringVar(&f.Mode, "mode", "", "mode of the rows: builtin or import")
	cmd.Flags().StringVar(&f.ProfileID, "profile-id", "", "profile of the job")
	cmd.Flags().StringVar(&f.ComponentID, "component", "", "component ID of the job")
	return cmd
}

func (a *app) runCmd() *cobra.Command {
	t := ci.Tool{}
	cmd := &cobra.Command{
		Use:   "run [flags] -- command [args...]",
		Short: "run a profile tool within the profile timeout and record its run",
		Long: `run executes a tool of a builtin profile and records the run in the scan manifest of the job. A
tool that runs out of time is recorded as timeout and partial, a crash or a missing report as failed;
only a finished report is success or findings and complete. The tool outcome does not fail the step:
the run is uploaded with the package.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() != 0 {
				return errors.New("expected flags -- command [args...]")
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			t.Command = args
			if t.ID == "" {
				t.ID = t.ProfileID + "-" + t.Kind
			}
			r := &ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store, Stdout: os.Stdout, Stderr: os.Stderr, Now: time.Now}
			run, err := r.Run(cmd.Context(), &t)
			if err != nil {
				return err
			}
			ci.PrintRun(a.stdout, run)
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&t.ID, "id", "", "tool run ID, unique in the job (default <profile-id>-<kind>)")
	fl.StringVar(&t.ProfileID, "profile-id", "", "builtin profile of the tool")
	fl.StringVar(&t.ComponentID, "component", "", "component ID when the profile checks several components")
	fl.StringVar(&t.Kind, "kind", "", "report kind: code, sast, sca, secrets, iac, image, junit, coverage, docs, openapi")
	fl.StringVar(&t.Format, "format", "", "report format, for example sarif or junit_xml")
	fl.StringVar(&t.FormatVersion, "format-version", "1", "version of the report format")
	fl.StringVar(&t.Producer, "producer", "", "tool name")
	fl.StringVar(&t.ProducerVersion, "producer-version", "", "tool version")
	fl.StringVar(&t.Report, "report", "", "file the tool writes its report to")
	fl.DurationVar(&t.Timeout, "timeout", _defaultTimeout, "time limit of the tool")
	fl.IntSliceVar(&t.FindingsCodes, "findings-exit-code", []int{1}, "exit codes that mean findings in a finished report")
	fl.StringVar(&t.ConfigFile, "config-file", "", "tool configuration, recorded by its hash")
	fl.StringVar(&t.RulesHash, "rules-hash", "", "SHA-256 of the rule set")
	fl.StringVar(&t.DBVersion, "db-version", "", "version of the vulnerability database")
	for _, name := range []string{"profile-id", "kind", "format", "producer", "producer-version", "report"} {
		_ = cmd.MarkFlagRequired(name) // the flags are declared just above
	}
	return cmd
}

func (a *app) importCmd() *cobra.Command {
	var (
		in    ci.Import
		specs []string
	)
	cmd := &cobra.Command{
		Use:   "import --report kind:format:path [--report ...]",
		Short: "record ready reports of the client pipeline",
		Long: `import records reports that the client pipeline already produces. Reports from the artifacts of
another job carry the pipeline ID and the SHA; pass --producer-job when the step knows the job that
produced them. A missing or empty report is recorded as failed, never as a clean result.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, raw := range specs {
				spec, err := ci.ParseReportSpec(raw)
				if err != nil {
					return err
				}
				in.Reports = append(in.Reports, spec)
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			im := &ci.Importer{Job: &s.job, Profile: s.profile, Store: s.store, Now: time.Now}
			runs, err := im.Add(&in)
			if err != nil {
				return err
			}
			for i := range runs {
				ci.PrintRun(a.stdout, &runs[i])
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringArrayVar(&specs, "report", nil, "report as kind:format:path, for example sast:sarif:gl-sast-report.sarif")
	fl.StringVar(&in.Producer, "producer", "client", "tool that produced the reports")
	fl.StringVar(&in.ProducerVersion, "producer-version", "unknown", "version of that tool")
	fl.StringVar(&in.FormatVersion, "format-version", "1", "version of the report format")
	fl.StringVar(&in.ProducerJob, "producer-job", "", "ID of the job that produced the reports")
	_ = cmd.MarkFlagRequired("report") // the flag is declared just above
	return cmd
}

func (a *app) uploadCmd() *cobra.Command {
	var (
		wait time.Duration
		save string
	)
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "pack the runs of the job, upload the package and print the intake result",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			s, err := a.session(ctx, false)
			if err != nil {
				return err
			}
			ci.PrintJob(a.stdout, health.CommitSHA(), &s.job, s.profile)
			p := &ci.Packer{Job: &s.job, Store: s.store, Version: health.CommitSHA(), Now: time.Now}
			pkg, err := p.Build(limits(s.profile))
			if err != nil {
				return err
			}
			ci.PrintPackage(a.stdout, pkg, s.profile)
			if save != "" {
				if err := os.WriteFile(save, pkg.Data, 0o600); err != nil {
					return fmt.Errorf("save package: %w", err)
				}
			}
			accepted, err := s.client.Upload(ctx, pkg.Data)
			if err != nil {
				return err
			}
			ci.PrintAccepted(a.stdout, accepted)
			receipt, err := s.client.Await(ctx, accepted, wait)
			if err != nil {
				return err
			}
			ci.PrintReceipt(a.stdout, receipt, s.client.ResultURL(receipt))
			if receipt.Status == ciapi.ReceiptStatusRejected {
				reason := "rejected"
				if receipt.ReasonCode != nil {
					reason = *receipt.ReasonCode
				}
				return &ci.RefusedError{Code: reason, Detail: "receipt " + receipt.Id.String()}
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", _defaultWait, "how long to wait for the intake result")
	cmd.Flags().StringVar(&save, "save", "", "also write the package to this file")
	return cmd
}

func (a *app) fixturesCmd() *cobra.Command {
	fx := ci.Fixtures{Stdout: os.Stdout, Stderr: os.Stderr}
	cmd := &cobra.Command{
		Use:   "fixtures DIR -- command [args...]",
		Short: "run a profile on its rule fixtures and compare the findings",
		Long: `fixtures runs the profile command on every case of a profile fixture directory and compares rule,
file and line of the findings with the expected list. A profile keeps its cases in its own
subdirectory, fixtures/<profile-id>/<case>/:

  src/          code checked by the profile; the command runs here
  expected.txt  expected findings, one per line: <rule> <path>:<line>; the path is relative to src/,
                # starts a comment

The command writes a SARIF report to the file named by CRAFTPULSE_REPORT. A missing or extra finding
fails the command.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() != 1 {
				return errors.New("expected DIR -- command [args...]")
			}
			fx.Dir, fx.Command = args[0], args[1:]
			results, err := fx.Run(cmd.Context())
			if err != nil {
				return err
			}
			return ci.PrintFixtures(a.stdout, results)
		},
	}
	cmd.Flags().DurationVar(&fx.Timeout, "timeout", _defaultTimeout, "time limit of one case")
	return cmd
}

// limits are the package limits of the profile; a profile without them falls back to NFR-015.
func limits(p *ciapi.CiCheckProfile) envelope.Limits {
	l := envelope.Limits{
		MaxPackageBytes: int64(p.Limits.MaxPackageBytes), MaxUnpackedBytes: int64(p.Limits.MaxUnpackedBytes), MaxFiles: p.Limits.MaxFiles,
	}
	if l.MaxPackageBytes <= 0 || l.MaxUnpackedBytes <= 0 || l.MaxFiles <= 0 {
		return envelope.DefaultLimits
	}
	return l
}
