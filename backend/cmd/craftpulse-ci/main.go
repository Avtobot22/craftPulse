// Command craftpulse-ci is the craftPulse step in the client pipeline: it reads the check profile of the
// project, runs profile tools or collects ready reports, packs them and uploads the package (TZ 10.5.2).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Avtobot22/craftPulse/backend/internal/ci"
	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
	"github.com/Avtobot22/craftPulse/backend/pkg/secrets"
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
	secretScan     *ci.SecretScan
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string, cfg ci.ClientConfig) int {
	redactedValues := []string{getenv(ci.TokenEnv)}
	var secretScan *ci.SecretScan
	var out, errs bytes.Buffer
	originalOut, originalErr := stdout, stderr
	if getenv("CRAFTPULSE_SECRETS") == "1" && (len(args) != 1 || (args[0] != "version" && args[0] != "secrets-info")) {
		repo := getenv("CI_PROJECT_DIR")
		if repo == "" {
			repo = "."
		}
		scanCtx, cancel := context.WithTimeout(ctx, _defaultTimeout)
		var err error
		secretScan, err = ci.ScanSecrets(scanCtx, repo, baseSHA(getenv), getenv("CI_COMMIT_SHA"))
		cancel()
		if err != nil {
			fmt.Fprintln(stderr, "craftpulse-ci: secrets scan failed; no output or package released")
			return _exitFailed
		}
		stdout, stderr = &out, &errs
		defer func() {
			for _, pair := range []struct {
				to   io.Writer
				data []byte
			}{{originalOut, out.Bytes()}, {originalErr, errs.Bytes()}} {
				safe, err := secretScan.Scrubber.Redact(pair.data)
				if err == nil {
					_, _ = pair.to.Write(safe)
				}
			}
		}()
	}
	a := &app{secretScan: secretScan, stdout: logger.Redact(stdout, redactedValues), stderr: logger.Redact(stderr, redactedValues), getenv: getenv, client: cfg}
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
	root.AddCommand(a.versionCmd(), a.profileCmd(), a.runCmd(), a.importCmd(), a.uploadCmd(), a.fixturesCmd(), a.codeCmd(), a.sastCmd(), a.trivyCmd(), a.trivyDBCmd(), a.docsCmd(), a.secretsCmd(), a.secretsInfoCmd())
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
	if a.secretScan != nil {
		s.store.Scrubber = a.secretScan.Scrubber
	}
	if !fresh {
		var ok bool
		if s.profile, ok, err = s.store.Profile(); err != nil {
			return nil, err
		} else if ok {
			return s, nil
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
			r := &ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store, Stdout: a.stdout, Stderr: a.stderr, Now: time.Now}
			// A tool may print newly generated credentials unknown to the repository scanner.
			// The secrets image publishes summaries only, never arbitrary tool streams.
			if a.secretScan != nil {
				r.Stdout, r.Stderr = io.Discard, io.Discard
			}
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

func (a *app) codeCmd() *cobra.Command {
	var (
		dir, component string
		timeout        time.Duration
		fixture        bool
	)
	cmd := &cobra.Command{
		Use:   "code",
		Short: "run the code profile of the image on the components of the job",
		Long: `code runs the code profile of the image on every component of its builtin code rows: the linter of
the profile language (golangci-lint for Go, Ruff for Python, ESLint with typescript-eslint for JS/TS)
gives findings in SARIF, Lizard the functions in CSV and jscpd the copies in JSON, three tool runs per
component within one time limit. Dependency and build directories and generated files are excluded,
test code is marked separately, and a file the linter cannot parse is a skipped target of a partial
run. The linters use the configuration of the profile only. The profile directory defaults to
CRAFTPULSE_CODE_PROFILE of the image and the repository to CI_PROJECT_DIR.

With --fixture the profile checks the current directory and writes the reports named by
CRAFTPULSE_REPORT, CRAFTPULSE_FUNCTIONS_REPORT, CRAFTPULSE_CLONES_REPORT and CRAFTPULSE_SCOPE_REPORT:
the command of "craftpulse-ci fixtures".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				dir = a.getenv("CRAFTPULSE_CODE_PROFILE")
			}
			if dir == "" {
				return errors.New("pass --profile-dir or set CRAFTPULSE_CODE_PROFILE")
			}
			p, err := ci.LoadCodeProfile(dir)
			if err != nil {
				return err
			}
			if fixture {
				return a.codeFixture(cmd.Context(), p)
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			root := a.getenv("CI_PROJECT_DIR")
			if root == "" {
				root = "."
			}
			r := &ci.CodeRunner{
				Runner: &ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store, Stdout: a.stdout, Stderr: a.stderr, Now: time.Now},
				Code:   p, Root: root, Timeout: timeout,
			}
			runs, err := r.Run(cmd.Context(), ci.CodeRows(s.profile, p.ID, component))
			for i := range runs {
				ci.PrintRun(a.stdout, &runs[i])
			}
			return err
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&dir, "profile-dir", "", "directory of the code profile")
	fl.StringVar(&component, "component", "", "component ID; all code rows of the profile by default")
	fl.DurationVar(&timeout, "timeout", 0, "time limit of the profile on all components (default from the profile)")
	fl.BoolVar(&fixture, "fixture", false, "check the current directory for the fixtures command")
	return cmd
}

func (a *app) sastCmd() *cobra.Command {
	var (
		dir, component string
		timeout        time.Duration
		fixture        bool
	)
	cmd := &cobra.Command{
		Use:   "sast",
		Short: "run the SAST profile of the image on the components of the job",
		Long: `sast runs Opengrep with the rule set of the image on every component of its builtin sast rows and
records one tool run per component with a SARIF report. The report keeps the bare rule IDs, CWE and
severity of the rules and no lines of code. Files Opengrep could not parse or ran out of time on are
skipped targets, and the run is partial. Vendor and build directories of the profile and directories
starting with "." are excluded. The profile directory defaults to CRAFTPULSE_SAST_PROFILE of the image
and the repository to CI_PROJECT_DIR.

With --fixture the profile checks the current directory and writes the report named by CRAFTPULSE_REPORT:
the command of "craftpulse-ci fixtures".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				dir = a.getenv("CRAFTPULSE_SAST_PROFILE")
			}
			if dir == "" {
				return errors.New("pass --profile-dir or set CRAFTPULSE_SAST_PROFILE")
			}
			p, err := ci.LoadSASTProfile(dir)
			if err != nil {
				return err
			}
			if fixture {
				report := a.getenv(ci.FixtureReportEnv)
				if report == "" {
					return errors.New("--fixture runs under the fixtures command, which names the report file")
				}
				return p.Fixture(cmd.Context(), a.stdout, a.stderr, report)
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			root := a.getenv("CI_PROJECT_DIR")
			if root == "" {
				root = "."
			}
			r := &ci.SASTRunner{
				Runner: &ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store, Stdout: a.stdout, Stderr: a.stderr, Now: time.Now},
				SAST:   p, Root: root, Timeout: timeout,
			}
			runs, err := r.Run(cmd.Context(), ci.SASTRows(s.profile, p.ID, component))
			for i := range runs {
				ci.PrintRun(a.stdout, &runs[i])
			}
			return err
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&dir, "profile-dir", "", "directory of the SAST profile")
	fl.StringVar(&component, "component", "", "component ID; all sast rows of the profile by default")
	fl.DurationVar(&timeout, "timeout", 0, "time limit of the profile on all components (default from the profile)")
	fl.BoolVar(&fixture, "fixture", false, "check the current directory for the fixtures command")
	return cmd
}

func (a *app) trivyCmd() *cobra.Command {
	var (
		dir, cacheDir, component string
		timeout                  time.Duration
		fixture                  bool
	)
	cmd := &cobra.Command{
		Use:   "trivy",
		Short: "run the Trivy profile of the image on the sca and iac rows of the job",
		Long: `trivy runs Trivy with the vulnerability database and checks of the image, without a network, on every
builtin sca and iac row of its profile and records one tool run per row with the JSON report of Trivy.
SCA reads the lockfiles of Go, Python and JS/TS and marks the package instances that the lockfiles of
the base commit (CI_MERGE_REQUEST_DIFF_BASE_SHA or CI_COMMIT_BEFORE_SHA) do not have. The sca run
records the version of the database and the time it was published; without a database it fails.
IaC checks the configuration kinds of the profile; the lines of configuration are dropped from the
report and every misconfiguration names its resource. Trivy reads the configuration and ignore file of
the profile, never those of the repository. The profile directory defaults to CRAFTPULSE_TRIVY_PROFILE,
the database to CRAFTPULSE_TRIVY_CACHE of the image and the repository to CI_PROJECT_DIR.

With --fixture the profile checks the current directory and writes its facts to the file named by
CRAFTPULSE_FACTS_REPORT: the command of "craftpulse-ci fixtures".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, cache, err := a.trivyProfile(dir, cacheDir)
			if err != nil {
				return err
			}
			if fixture {
				facts := a.getenv(ci.FixtureFactsEnv)
				if facts == "" {
					return errors.New("--fixture runs under the fixtures command, which names the facts file")
				}
				return p.Fixture(cmd.Context(), a.stdout, a.stderr, cache, facts)
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			root := a.getenv("CI_PROJECT_DIR")
			if root == "" {
				root = "."
			}
			r := &ci.TrivyRunner{
				Runner: &ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store, Stdout: a.stdout, Stderr: a.stderr, Now: time.Now},
				Trivy:  p, Root: root, CacheDir: cache, BaseSHA: baseSHA(a.getenv), Timeout: timeout,
			}
			runs, err := r.Run(cmd.Context(), ci.TrivyRows(s.profile, p.ID, component))
			for i := range runs {
				ci.PrintRun(a.stdout, &runs[i])
			}
			return err
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&dir, "profile-dir", "", "directory of the Trivy profile")
	fl.StringVar(&cacheDir, "cache-dir", "", "Trivy cache directory with the vulnerability database")
	fl.StringVar(&component, "component", "", "component ID; all sca and iac rows of the profile by default")
	fl.DurationVar(&timeout, "timeout", 0, "time limit of the profile on all rows (default from the profile)")
	fl.BoolVar(&fixture, "fixture", false, "check the current directory for the fixtures command")
	return cmd
}

func (a *app) docsCmd() *cobra.Command {
	var (
		dir     string
		timeout time.Duration
		fixture bool
	)
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "check the documentation roots of the job with the docs profile of the image",
		Long: `docs checks every documentation root of the project profile whose kind has a builtin row of the
profile and records one tool run per root and kind with a SARIF report and a docs_json inventory.
docs: Markdown is parsed with goldmark; a local link to a file that does not exist with exactly that
name, or to an anchor no heading of the target makes, is a finding at the line of the link; external
links are not checked. The inventory lists every Markdown and OpenAPI document of the root with its
path, the SHA of the job, the content hash, the title and the owner from front matter. openapi:
Spectral checks the specifications with the ruleset of the profile; a parse or schema error makes a
specification invalid, other results break a rule. The profile directory defaults to
CRAFTPULSE_DOCS_PROFILE of the image and the repository to CI_PROJECT_DIR.

With --fixture the profile checks the current directory and writes the files named by CRAFTPULSE_REPORT
and CRAFTPULSE_FACTS_REPORT: the command of "craftpulse-ci fixtures".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				dir = a.getenv("CRAFTPULSE_DOCS_PROFILE")
			}
			if dir == "" {
				return errors.New("pass --profile-dir or set CRAFTPULSE_DOCS_PROFILE")
			}
			p, err := ci.LoadDocsProfile(dir)
			if err != nil {
				return err
			}
			if fixture {
				report, facts := a.getenv(ci.FixtureReportEnv), a.getenv(ci.FixtureFactsEnv)
				if report == "" || facts == "" {
					return errors.New("--fixture runs under the fixtures command, which names the report files")
				}
				return p.Fixture(cmd.Context(), a.stdout, a.stderr, report, facts)
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			root := a.getenv("CI_PROJECT_DIR")
			if root == "" {
				root = "."
			}
			r := &ci.DocsRunner{
				Runner: &ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store, Stdout: a.stdout, Stderr: a.stderr, Now: time.Now},
				Docs:   p, Root: root, Timeout: timeout,
			}
			runs, err := r.Run(cmd.Context(), ci.DocsRows(s.profile, p.ID), ci.DocsRoots(s.profile))
			for i := range runs {
				ci.PrintRun(a.stdout, &runs[i])
			}
			return err
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&dir, "profile-dir", "", "directory of the docs profile")
	fl.DurationVar(&timeout, "timeout", 0, "time limit of the profile on all roots (default from the profile)")
	fl.BoolVar(&fixture, "fixture", false, "check the current directory for the fixtures command")
	return cmd
}

func (a *app) trivyDBCmd() *cobra.Command {
	var cacheDir, repository string
	cmd := &cobra.Command{
		Use:   "trivy-db",
		Short: "replace the vulnerability database of the Trivy profile from an OCI repository",
		Long: `trivy-db downloads the Trivy vulnerability database from the repository (--repository or
CRAFTPULSE_TRIVY_DB_REPOSITORY): the public ghcr.io/aquasecurity/trivy-db:2 or an internal mirror,
by tag or digest. The new database replaces the current one only after it is downloaded and checked;
a failed update keeps the current database and its date and fails the command.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cacheDir == "" {
				cacheDir = a.getenv("CRAFTPULSE_TRIVY_CACHE")
			}
			if repository == "" {
				repository = a.getenv("CRAFTPULSE_TRIVY_DB_REPOSITORY")
			}
			if cacheDir == "" {
				return errors.New("pass --cache-dir or set CRAFTPULSE_TRIVY_CACHE")
			}
			r := &ci.Runner{Stdout: a.stdout, Stderr: a.stderr, Now: time.Now}
			db, err := r.UpdateTrivyDB(cmd.Context(), cacheDir, repository)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "trivy-db %s published %s from %s\n", db.Version, db.PublishedAt.Format(time.RFC3339), db.Repository)
			return nil
		},
	}
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Trivy cache directory (default CRAFTPULSE_TRIVY_CACHE)")
	cmd.Flags().StringVar(&repository, "repository", "", "OCI repository of the database (default CRAFTPULSE_TRIVY_DB_REPOSITORY)")
	return cmd
}

func (a *app) trivyProfile(dir, cacheDir string) (*ci.TrivyProfile, string, error) {
	if dir == "" {
		dir = a.getenv("CRAFTPULSE_TRIVY_PROFILE")
	}
	if cacheDir == "" {
		cacheDir = a.getenv("CRAFTPULSE_TRIVY_CACHE")
	}
	if dir == "" || cacheDir == "" {
		return nil, "", errors.New("pass --profile-dir and --cache-dir or set CRAFTPULSE_TRIVY_PROFILE and CRAFTPULSE_TRIVY_CACHE")
	}
	p, err := ci.LoadTrivyProfile(dir)
	return p, cacheDir, err
}

// baseSHA is the commit the job compares head with: the merge request base or the previous push, none for a
// new branch.
func baseSHA(getenv func(string) string) string {
	base := getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA")
	if base == "" {
		base = getenv("CI_COMMIT_BEFORE_SHA")
	}
	if strings.Trim(base, "0") == "" {
		return ""
	}
	return base
}

func (a *app) codeFixture(ctx context.Context, p *ci.CodeProfile) error {
	out := ci.CodeReports{Lint: a.getenv(ci.FixtureReportEnv), Functions: a.getenv(ci.FixtureFunctionsEnv), Clones: a.getenv(ci.FixtureClonesEnv)}
	scope := a.getenv(ci.FixtureScopeEnv)
	if out.Lint == "" || out.Functions == "" || out.Clones == "" || scope == "" {
		return errors.New("--fixture runs under the fixtures command, which names the report files")
	}
	return p.Fixture(ctx, a.stdout, a.stderr, out, scope)
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
			if in.SourceRoot == "" {
				in.SourceRoot = a.getenv("CI_PROJECT_DIR")
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
	fl.StringVar(&in.SourceRoot, "source-root", "", "absolute checkout the tool ran in (default CI_PROJECT_DIR)")
	fl.StringVar(&in.SourceDir, "source-dir", "", "directory of the repository the relative paths of the reports start from, for example src/main/java")
	_ = cmd.MarkFlagRequired("report") // the flag is declared just above
	return cmd
}

func (a *app) uploadCmd() *cobra.Command {
	var (
		wait   time.Duration
		save   string
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "pack the runs of the job, upload the package and print the intake result",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRun && save == "" {
				return errors.New("--dry-run requires --save")
			}
			ctx := cmd.Context()
			s, err := a.session(ctx, false)
			if err != nil {
				return err
			}
			ci.PrintJob(a.stdout, health.CommitSHA(), &s.job, s.profile)
			if a.secretScan != nil {
				if err := a.secretScan.Record(&ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store}, a.getenv("CRAFTPULSE_COMPONENT_ID")); err != nil {
					return err
				}
			}
			p := &ci.Packer{Job: &s.job, Store: s.store, Version: health.CommitSHA(), Now: time.Now}
			if a.secretScan != nil {
				p.Scrubber = a.secretScan.Scrubber
			}
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
			if dryRun {
				return nil
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "build and check the saved package without uploading")
	return cmd
}

func (a *app) fixturesCmd() *cobra.Command {
	fx := ci.Fixtures{Stdout: a.stdout, Stderr: a.stderr}
	if a.secretScan != nil {
		fx.Stdout, fx.Stderr = io.Discard, io.Discard
	}
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

func (a *app) secretsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "secrets", Short: "scan head and diff with the pinned Gitleaks profile", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.secretScan == nil {
				return errors.New("use the secrets profile image (CRAFTPULSE_SECRETS=1)")
			}
			s, err := a.session(cmd.Context(), false)
			if err != nil {
				return err
			}
			if err := a.secretScan.Record(&ci.Runner{Job: &s.job, Profile: s.profile, Store: s.store}, a.getenv("CRAFTPULSE_COMPONENT_ID")); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "gitleaks %s: head=%d diff=%d history_truncated=%t\n", ci.GitleaksVersion, len(a.secretScan.Head), len(a.secretScan.Diff), a.secretScan.History.Truncated)
			return nil
		},
	}
}

func (a *app) secretsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "secrets-info", Short: "print the pinned secrets profile identity", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return json.NewEncoder(a.stdout).Encode(map[string]any{"profile_id": ci.SecretsProfileID, "gitleaks_version": ci.GitleaksVersion, "rules_hash": secrets.RulesHash()})
		},
	}
}
