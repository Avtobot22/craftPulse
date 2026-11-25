package ci

import (
	"bytes"
	"context"
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

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/secrets"
)

const (
	SecretsProfileID = secrets.ProfileID
	GitleaksVersion  = secrets.GitleaksVersion
	_gitOutputLimit  = 32 << 20
	_scanByteLimit   = 256 << 20
)

var _addedHunk = regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// SecretScan holds safe findings and an ephemeral scrubber; it must never be serialized.
type SecretScan struct {
	Head, Diff            []secrets.Finding
	History               envelope.ScanHistory
	Scrubber              *secrets.Scrubber
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
	detector, err := secrets.New(ctx)
	if err != nil {
		return nil, err
	}
	s := &SecretScan{Head: []secrets.Finding{}, Diff: []secrets.Finding{}, Scrubber: detector.Scrubber}
	s.RulesHash, s.StartedAt = secrets.RulesHash(), startedAt
	var total int
	scan := func(commit, path string) ([]secrets.Finding, error) {
		content, err := secretGit(ctx, repo, "cat-file", "blob", commit+":"+path)
		if err != nil {
			return nil, err
		}
		total += len(content)
		if total > _scanByteLimit {
			return nil, errors.New("secrets scan exceeds byte limit")
		}
		return detector.Scan(ctx, content, path, commit)
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
		s.Head = append(s.Head, found...)
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
	s.History.ToAt = secretCommitTime(ctx, repo, head)
	s.History.FromAt = s.History.ToAt
	if s.History.FromSha != head {
		s.History.FromAt = secretCommitTime(ctx, repo, s.History.FromSha)
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
			var added []secrets.Finding
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
			s.Diff = append(s.Diff, added...)
		}
	}
	// Paths can themselves contain a detected value; scrub every public string too.
	for _, findings := range [][]secrets.Finding{s.Head, s.Diff} {
		for i := range findings {
			safe, err := s.Scrubber.Redact([]byte(findings[i].File))
			if err != nil {
				return nil, err
			}
			findings[i].File = string(safe)
		}
	}
	s.FinishedAt = time.Now().UTC()
	return s, nil
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

func secretCommitTime(ctx context.Context, repo, sha string) *time.Time {
	out, err := secretGit(ctx, repo, "show", "-s", "--format=%cI", sha, "--")
	if err != nil {
		return nil
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
	if err != nil {
		return nil
	}
	return new(at.UTC())
}

// Record writes only the safe reports. Repeated scans still recreate the in-memory scrubber.
func (s *SecretScan) Record(r *Runner, component string) error {
	r.Store.Scrubber = s.Scrubber
	for _, scope := range []struct {
		name     string
		findings []secrets.Finding
		history  envelope.ScanHistory
	}{
		{"head", s.Head, envelope.ScanHistory{FromSha: r.Job.SHA, ToSha: r.Job.SHA, FromAt: s.History.ToAt, ToAt: s.History.ToAt}},
		{"diff", s.Diff, s.History},
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
		if err := scrubMetadata(s.Scrubber, run); err != nil {
			return err
		}
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
