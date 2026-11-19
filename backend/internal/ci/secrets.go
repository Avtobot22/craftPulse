package ci

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/logging"
	"github.com/zricethezav/gitleaks/v8/report"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	SecretsProfileID = "gitleaks-default"
	GitleaksVersion  = "8.30.1"
	_gitOutputLimit  = 32 << 20
	_scanByteLimit   = 256 << 20
)

var (
	//go:embed profiles/gitleaks-default/rules.toml
	_secretRules string
	//go:embed profiles/gitleaks-default/profile.json
	_secretProfile []byte
	_addedHunk     = regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
)

func init() {
	// Gitleaks trace messages can contain rejected secret matches. Never enable its logger.
	logging.Logger = zerolog.Nop()
}

// SecretFinding is the entire public finding. Evidence and values never leave the scanner.
// Field names keep the Gitleaks JSON report format; fingerprint excludes line and commit.
type SecretFinding struct {
	RuleID      string `json:"RuleID"`      //nolint:tagliatelle // Gitleaks format
	File        string `json:"File"`        //nolint:tagliatelle // Gitleaks format
	StartLine   int    `json:"StartLine"`   //nolint:tagliatelle // Gitleaks format
	Commit      string `json:"Commit"`      //nolint:tagliatelle // Gitleaks format
	Fingerprint string `json:"Fingerprint"` //nolint:tagliatelle // Gitleaks format
}

// SecretScan holds safe findings and an ephemeral scrubber; it must never be serialized.
type SecretScan struct {
	Head, Diff            []SecretFinding
	History               envelope.ScanHistory
	Scrubber              *SecretScrubber
	RulesHash             string
	Files                 int
	StartedAt, FinishedAt time.Time
}

// ScanSecrets scans blobs at head and additions in every available commit in base..head.
// Missing base or shallow boundaries restrict the checked range and mark it partial.
func ScanSecrets(ctx context.Context, repo, base, head string) (*SecretScan, error) {
	if !_sha.MatchString(head) || (base != "" && !_sha.MatchString(base)) {
		return nil, errors.New("secrets scan requires full lowercase commit SHAs")
	}
	startedAt := time.Now().UTC()
	cfg, minFragment, err := secretConfig()
	if err != nil {
		return nil, err
	}
	d := detect.NewDetectorContext(ctx, cfg)
	d.IgnoreGitleaksAllow = true
	s := &SecretScan{Head: []SecretFinding{}, Diff: []SecretFinding{}, Scrubber: NewSecretScrubber(minFragment)}
	s.RulesHash, s.StartedAt = SecretsRulesHash(), startedAt
	var total int
	scan := func(commit, path string) ([]report.Finding, error) {
		content, err := secretGit(ctx, repo, "cat-file", "blob", commit+":"+path)
		if err != nil {
			return nil, err
		}
		total += len(content)
		if total > _scanByteLimit {
			return nil, errors.New("secrets scan exceeds byte limit")
		}
		found := d.DetectContext(ctx, detect.Fragment{Raw: string(content), FilePath: path, CommitSHA: commit, StartLine: 1}) //nolint:staticcheck // pinned v8 API still requires detect.Fragment; sources.Fragment is a distinct type
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return found, nil
	}
	tree, err := secretGit(ctx, repo, "ls-tree", "-rz", "--full-tree", head)
	if err != nil {
		return nil, err
	}
	for _, entry := range bytes.Split(tree, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		meta, path, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			return nil, errors.New("invalid Git tree")
		}
		if !bytes.Contains(meta, []byte(" blob ")) {
			return nil, errors.New("secrets scan does not cover submodules")
		}
		found, err := scan(head, string(path))
		if err != nil {
			return nil, err
		}
		s.Files++
		s.Head = append(s.Head, s.collect(found)...)
	}
	s.History = envelope.ScanHistory{FromSha: head, ToSha: head, Truncated: true}
	rangeSpec := head
	if base != "" {
		if _, err := secretGit(ctx, repo, "cat-file", "-e", base+"^{commit}"); err == nil {
			rangeSpec = base + ".." + head
			s.History.FromSha, s.History.Truncated = base, false
		}
	}
	commits, err := secretGit(ctx, repo, "rev-list", "--reverse", rangeSpec, "--")
	if err != nil {
		return nil, err
	}
	list := strings.Fields(string(commits))
	if s.History.Truncated && len(list) > 0 {
		s.History.FromSha = list[0]
	}
	shallow, err := secretGit(ctx, repo, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	// Conservatively report any shallow checkout as truncated, even when base happens to exist.
	if strings.TrimSpace(string(shallow)) == "true" {
		s.History.Truncated = true
	}
	for _, commit := range list {
		paths, err := secretGit(ctx, repo, "diff-tree", "--root", "--no-commit-id", "--diff-merges=first-parent", "-r", "--no-renames", "--name-only", "--diff-filter=ACMT", "-z", commit, "--")
		if err != nil {
			return nil, err
		}
		for _, path := range bytes.Split(paths, []byte{0}) {
			if len(path) == 0 {
				continue
			}
			patch, err := secretGit(ctx, repo, "show", "--format=", "--text", "--diff-merges=first-parent", "--no-ext-diff", "--no-textconv", "--no-renames", "--unified=0", commit, "--", string(path))
			if err != nil {
				return nil, err
			}
			found, err := scan(commit, string(path))
			if err != nil {
				return nil, err
			}
			var added []report.Finding
			for i := range found {
				f := &found[i]
				for _, h := range _addedHunk.FindAllSubmatch(patch, -1) {
					start, _ := strconv.Atoi(string(h[1]))
					count := 1
					if len(h[2]) > 0 {
						count, _ = strconv.Atoi(string(h[2]))
					}
					if count > 0 && f.StartLine <= start+count-1 && f.EndLine >= start {
						added = append(added, *f)
						break
					}
				}
			}
			s.Diff = append(s.Diff, s.collect(added)...)
		}
	}
	// Paths can themselves contain a detected value; scrub every public string too.
	for _, findings := range [][]SecretFinding{s.Head, s.Diff} {
		for i := range findings {
			findings[i].File = string(s.Scrubber.clean([]byte(findings[i].File)))
		}
	}
	s.FinishedAt = time.Now().UTC()
	return s, nil
}

func secretConfig() (config.Config, int, error) {
	var policy struct {
		MinFragmentLength int `json:"min_fragment_length"`
	}
	if err := json.Unmarshal(_secretProfile, &policy); err != nil || policy.MinFragmentLength != 8 {
		return config.Config{}, 0, errors.New("invalid secrets policy")
	}
	v := viper.New()
	v.SetConfigType("toml")
	if err := v.ReadConfig(strings.NewReader(config.DefaultConfig + "\n" + _secretRules)); err != nil {
		return config.Config{}, 0, errors.New("invalid secrets rules")
	}
	var vc config.ViperConfig
	if err := v.Unmarshal(&vc); err != nil {
		return config.Config{}, 0, errors.New("invalid secrets rules")
	}
	cfg, err := vc.Translate()
	if err != nil {
		return config.Config{}, 0, errors.New("invalid secrets rules")
	}
	return cfg, policy.MinFragmentLength, nil
}

func (s *SecretScan) collect(found []report.Finding) []SecretFinding {
	out := make([]SecretFinding, 0, len(found))
	for i := range found {
		f := &found[i]
		s.Scrubber.add(f.Secret)
		secretHash := sha256.Sum256([]byte(f.Secret))
		// Hash components independently to avoid delimiter ambiguity; no plaintext is in the identity.
		identity, _ := json.Marshal([]string{f.RuleID, f.File, hex.EncodeToString(secretHash[:])})
		fingerprint := sha256.Sum256(identity)
		out = append(out, SecretFinding{f.RuleID, f.File, f.StartLine, f.Commit, hex.EncodeToString(fingerprint[:])})
	}
	return out
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > _gitOutputLimit {
		return 0, errors.New("git output exceeds scan limit")
	}
	return b.Buffer.Write(p)
}

func secretGit(ctx context.Context, repo string, args ...string) ([]byte, error) {
	absolute, err := filepath.Abs(repo)
	if err != nil {
		return nil, errors.New("cannot resolve scan repository")
	}
	// The CI checkout may belong to the runner helper UID. Trust only the requested directory.
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "-C", absolute, "-c", "safe.directory=" + absolute, "-c", "core.hooksPath=/dev/null", "-c", "diff.external="}, args...)...) //nolint:gosec // arguments are fixed commands and validated SHAs
	var out cappedBuffer
	cmd.Stdout, cmd.Stderr, cmd.Env = &out, io.Discard, toolEnv("GIT_NO_REPLACE_OBJECTS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_LITERAL_PATHSPECS=1")
	cmd.WaitDelay = _killGrace
	if err := cmd.Run(); err != nil {
		return nil, errors.New("git secrets scan failed (repository, range or resource limit)")
	}
	return out.Bytes(), nil
}

// Record writes only the safe reports. Repeated scans still recreate the in-memory scrubber.
func (s *SecretScan) Record(r *Runner, component string) error {
	for _, scope := range []struct {
		name     string
		findings []SecretFinding
		history  envelope.ScanHistory
	}{
		{"head", s.Head, envelope.ScanHistory{FromSha: r.Job.SHA, ToSha: r.Job.SHA}}, {"diff", s.Diff, s.History},
	} {
		tool := &Tool{ID: SecretsProfileID + "-" + scope.name, ProfileID: SecretsProfileID, ComponentID: component, Kind: "secrets", Format: "gitleaks_json", Producer: "gitleaks", ProducerVersion: GitleaksVersion, RulesHash: s.RulesHash}
		entry, err := r.entry(tool)
		if err != nil {
			return err
		}
		run, err := r.toolRun(tool, entry)
		if err != nil {
			return err
		}
		run.StartedAt, run.FinishedAt = s.StartedAt, s.FinishedAt
		run.ExitClass, run.Completeness = envelope.ScanToolRunExitClassSuccess, envelope.ScanToolRunCompletenessComplete
		if len(scope.findings) > 0 {
			run.ExitClass = envelope.ScanToolRunExitClassFindings
		}
		run.History = &scope.history
		if scope.history.Truncated {
			run.Completeness = envelope.ScanToolRunCompletenessPartial
		}
		if scope.name == "head" {
			run.ScannedCount = &s.Files
		}
		data, err := json.Marshal(scope.findings)
		if err != nil {
			return fmt.Errorf("encode safe secrets report: %w", err)
		}
		name := "files/reports/" + tool.ID + "/report.json"
		run.Reports = []envelope.ScanReport{{ReportKind: envelope.ScanReportReportKindSecrets, ReportFormat: envelope.ScanReportReportFormatGitleaksJson, FormatVersion: "1", File: envelope.FileRef{Path: name, MediaType: "application/json"}}}
		existing, err := r.Store.Runs()
		if err != nil {
			return err
		}
		exists := false
		for i := range existing {
			if existing[i].Id != tool.ID {
				continue
			}
			// Idempotence is valid only for the same SHA, range, profile and configuration.
			previous := existing[i]
			previous.StartedAt, previous.FinishedAt = run.StartedAt, run.FinishedAt
			if !reflect.DeepEqual(previous, *run) {
				return errors.New("state contains a different secrets scan scope; use a fresh work directory")
			}
			exists = true
			break
		}
		if exists {
			continue
		}
		if err := r.Store.write(name, data); err != nil {
			return err
		}
		if err := r.Store.AddRun(run, nil); err != nil {
			return err
		}
	}
	return nil
}

// SecretsRulesHash identifies pinned upstream defaults, local rules and the scrubbing policy.
func SecretsRulesHash() string {
	sum := sha256.Sum256([]byte(GitleaksVersion + "\n" + _secretRules + string(_secretProfile)))
	return hex.EncodeToString(sum[:])
}
