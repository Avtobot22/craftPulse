package ci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/secrets"
)

func fixtureMarker() string {
	sum := sha256.Sum256([]byte("craftpulse fixture marker, not a credential"))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:32]
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "Git fixture command failed")
	return strings.TrimSpace(string(data))
}

func fixtureCommit(t *testing.T, repo, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "config.go"), []byte(content), 0o600))
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "fixture")
	return fixtureGit(t, repo, "rev-parse", "HEAD")
}

func fixtureRepo(t *testing.T) (repo, base, head string) {
	t.Helper()
	repo = t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	base = fixtureCommit(t, repo, "package config\n// clean\n")
	head = fixtureCommit(t, repo, "package config\nconst defaultPartnerAPIKey = \""+fixtureMarker()+"\"\n")
	return
}

func TestSecrets_HeadDiffCleanRemovedAndFingerprint(t *testing.T) {
	repo, base, head := fixtureRepo(t)
	s, err := ScanSecrets(t.Context(), repo, base, head)
	require.NoError(t, err)
	require.Len(t, s.Head, 1)
	require.Len(t, s.Diff, 1)
	assert.Equal(t, "craftpulse-demo-secret", s.Head[0].RuleID)
	assert.Equal(t, "config.go", s.Head[0].File)
	assert.Equal(t, 2, s.Head[0].StartLine)
	assert.Equal(t, head, s.Diff[0].Commit)
	assert.Equal(t, base, s.History.FromSha)
	assert.Equal(t, head, s.History.ToSha)
	assert.False(t, s.History.Truncated)
	require.NotNil(t, s.History.FromAt)
	require.NotNil(t, s.History.ToAt)
	from, err := time.Parse(time.RFC3339, fixtureGit(t, repo, "show", "-s", "--format=%cI", base))
	require.NoError(t, err)
	to, err := time.Parse(time.RFC3339, fixtureGit(t, repo, "show", "-s", "--format=%cI", head))
	require.NoError(t, err)
	assert.True(t, s.History.FromAt.Equal(from))
	assert.True(t, s.History.ToAt.Equal(to))
	clean, err := ScanSecrets(t.Context(), repo, base, base)
	require.NoError(t, err)
	assert.Empty(t, clean.Head)
	assert.Empty(t, clean.Diff)
	// Dirty worktree content must not influence a scan of a named SHA.
	next := fixtureCommit(t, repo, "package config\n// adjacent edit\nconst defaultPartnerAPIKey = \""+fixtureMarker()+"\"\n")
	moved, err := ScanSecrets(t.Context(), repo, head, next)
	require.NoError(t, err)
	require.Len(t, moved.Head, 1)
	assert.Equal(t, s.Head[0].Fingerprint, moved.Head[0].Fingerprint)
	assert.Equal(t, 3, moved.Head[0].StartLine)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "config.go"), []byte("dirty clean file"), 0o600))
	atSHA, err := ScanSecrets(t.Context(), repo, base, head)
	require.NoError(t, err)
	assert.Equal(t, s.Head, atSHA.Head)
	// A value added then removed still needs scrubbing from imported reports.
	removed := fixtureCommit(t, repo, "package config\n// removed\n")
	history, err := ScanSecrets(t.Context(), repo, base, removed)
	require.NoError(t, err)
	assert.Empty(t, history.Head)
	require.NotEmpty(t, history.Diff)
	safe, err := history.Scrubber.Redact([]byte(fixtureMarker()))
	require.NoError(t, err)
	assert.NotContains(t, string(safe), fixtureMarker())
}

func TestSecrets_ShallowRange(t *testing.T) {
	repo, base, head := fixtureRepo(t)
	clone := filepath.Join(t.TempDir(), "shallow")
	fixtureGit(t, t.TempDir(), "clone", "--quiet", "--depth=1", "file://"+repo, clone)
	s, err := ScanSecrets(t.Context(), clone, base, head)
	require.NoError(t, err)
	assert.True(t, s.History.Truncated)
	assert.Equal(t, head, s.History.FromSha)
	assert.Equal(t, head, s.History.ToSha)
	require.NotNil(t, s.History.FromAt)
	assert.Equal(t, s.History.FromAt, s.History.ToAt)
	require.Len(t, s.Head, 1)
	require.Len(t, s.Diff, 1)
}

func TestSecrets_ScrubEscapedReportsAndAllPackageEntries(t *testing.T) {
	repo, base, head := fixtureRepo(t)
	s, err := ScanSecrets(t.Context(), repo, base, head)
	require.NoError(t, err)
	marker := fixtureMarker()
	var escaped strings.Builder
	for _, c := range marker {
		escaped.WriteString("&#" + strconv.Itoa(int(c)) + ";")
	}
	junit := []byte(`<testsuite name="` + marker + `"><testcase><failure>` + escaped.String() + ` fragment ` + marker[7:15] + `</failure></testcase></testsuite>`)
	safe, err := s.Scrubber.Redact(junit)
	require.NoError(t, err)
	var document any
	assert.NoError(t, xml.Unmarshal(safe, &document))
	assertNoFragments(t, safe, marker)
	var escapedJSON strings.Builder
	for _, c := range marker {
		fmt.Fprintf(&escapedJSON, "\\u00%02x", c)
	}
	safe, err = s.Scrubber.Redact([]byte(`{"evidence":"` + escapedJSON.String() + `"}`))
	require.NoError(t, err)
	assert.True(t, json.Valid(safe))
	assertNoFragments(t, safe, marker)
	job, err := JobFromEnv(gitlabEnv(map[string]string{"CI_COMMIT_SHA": head, "CRAFTPULSE_WORK_DIR": t.TempDir(), "CI_PROJECT_PATH": "demo/" + marker}))
	require.NoError(t, err)
	store := NewStore(job.WorkDir)
	var profile ciapi.CiCheckProfile
	require.NoError(t, json.Unmarshal([]byte(`{"entries":[{"component_id":"0193a5b0-7777-7000-8000-000000000001","path_prefix":"","language":"go","analysis_kind":"secrets","mode":"builtin","profile_id":"gitleaks-default","image_digest":"sha256:`+strings.Repeat("a", 64)+`"}]}`), &profile))
	require.NoError(t, s.Record(&Runner{Job: &job, Profile: &profile, Store: store}, ""))
	// Repeated commands rescan values, but do not duplicate tool reports.
	require.NoError(t, s.Record(&Runner{Job: &job, Profile: &profile, Store: store}, ""))
	file := filepath.Join(t.TempDir(), "junit.xml")
	require.NoError(t, os.WriteFile(file, junit, 0o600))
	im := Importer{Job: &job, Profile: &profile, Store: store, Now: time.Now}
	_, err = im.Add(&Import{Producer: "fixture", ProducerVersion: "1", FormatVersion: "1", Reports: []ReportSpec{{Kind: "junit", Format: "junit_xml", Path: file}}})
	require.NoError(t, err)
	p := Packer{Job: &job, Store: store, Scrubber: s.Scrubber, Version: "test", Now: time.Now}
	pkg, err := p.Build(envelope.DefaultLimits)
	require.NoError(t, err)
	gz, err := gzip.NewReader(bytes.NewReader(pkg.Data))
	require.NoError(t, err)
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		assertNoFragments(t, []byte(header.Name), marker)
		assertNoFragments(t, data, marker)
	}
	runs, err := store.Runs()
	require.NoError(t, err)
	require.Len(t, runs, 3)
	assert.Equal(t, s.RulesHash, *runs[0].RulesHash)
	p.Scrubber = nil
	_, err = p.Build(envelope.DefaultLimits)
	assert.ErrorContains(t, err, "fresh in-memory scrubber")
	// Safe scanner reports on disk never contain the value or fragments.
	require.NoError(t, filepath.WalkDir(job.WorkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.Contains(path, SecretsProfileID) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assertNoFragments(t, data, marker)
		}
		return nil
	}))
}

func assertNoFragments(t *testing.T, data []byte, marker string) {
	t.Helper()
	for i := 0; i+8 <= len(marker); i++ {
		assert.False(t, bytes.Contains(data, []byte(marker[i:i+8])), "secret fragment present")
	}
}

func TestSecrets_DiffCannotHideAdditionsAsBinary(t *testing.T) {
	for _, name := range []string{"attributes", "nul"} {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			fixtureGit(t, repo, "init", "--quiet")
			if name == "attributes" {
				require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("config.go -diff\n"), 0o600))
			}
			base := fixtureCommit(t, repo, "package config\n// clean\n")
			content := "package config\nconst defaultPartnerAPIKey = \"" + fixtureMarker() + "\"\n"
			if name == "nul" {
				content = "\x00" + content
			}
			added := fixtureCommit(t, repo, content)
			removed := fixtureCommit(t, repo, "package config\n// removed\n")
			scan, err := ScanSecrets(t.Context(), repo, base, removed)
			require.NoError(t, err)
			assert.Empty(t, scan.Head)
			require.Len(t, scan.Diff, 1)
			assert.Equal(t, added, scan.Diff[0].Commit)
			assert.Equal(t, 2, scan.Diff[0].StartLine)
			safe, err := scan.Scrubber.Redact([]byte(fixtureMarker()))
			require.NoError(t, err)
			assertNoFragments(t, safe, fixtureMarker())
		})
	}
}

func TestSecrets_RecordRejectsAChangedRange(t *testing.T) {
	repo, base, head := fixtureRepo(t)
	scan, err := ScanSecrets(t.Context(), repo, base, head)
	require.NoError(t, err)
	job, err := JobFromEnv(gitlabEnv(map[string]string{"CI_COMMIT_SHA": head, "CRAFTPULSE_WORK_DIR": t.TempDir()}))
	require.NoError(t, err)
	var profile ciapi.CiCheckProfile
	require.NoError(t, json.Unmarshal([]byte(`{"entries":[{"component_id":"0193a5b0-7777-7000-8000-000000000001","path_prefix":"","language":"go","analysis_kind":"secrets","mode":"builtin","profile_id":"gitleaks-default","image_digest":"sha256:`+strings.Repeat("a", 64)+`"}]}`), &profile))
	runner := &Runner{Job: &job, Profile: &profile, Store: NewStore(job.WorkDir)}
	require.NoError(t, scan.Record(runner, ""))
	require.NoError(t, scan.Record(runner, ""))
	changed, err := ScanSecrets(t.Context(), repo, head, head)
	require.NoError(t, err)
	assert.ErrorContains(t, changed.Record(runner, ""), "different secrets scan scope")
	runs, err := runner.Store.Runs()
	require.NoError(t, err)
	require.Len(t, runs, 2)
	for _, run := range runs {
		if strings.HasSuffix(run.Id, "-diff") {
			assert.Equal(t, base, run.History.FromSha)
		}
	}
}

func TestSecretScrubber_PreservesXMLNamespaces(t *testing.T) {
	s := secrets.NewScrubber(8)
	s.Add(fixtureMarker())
	content := []byte(`<testsuite xmlns="urn:fixture" xmlns:x="urn:other"><x:testcase name="` + fixtureMarker() + `"/></testsuite>`)
	for range 2 {
		safe, err := s.Redact(content)
		require.NoError(t, err)
		assertNoFragments(t, safe, fixtureMarker())
		decoder := xml.NewDecoder(bytes.NewReader(safe))
		for {
			token, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			if start, ok := token.(xml.StartElement); ok {
				seen := map[xml.Name]bool{}
				for _, attr := range start.Attr {
					require.False(t, seen[attr.Name], "duplicate XML namespace/attribute")
					seen[attr.Name] = true
				}
			}
		}
		content = safe
	}
}

func TestSecretScrubber_DecodesUTF8BOMReports(t *testing.T) {
	s := secrets.NewScrubber(8)
	s.Add(fixtureMarker())
	var entities, unicode strings.Builder
	for _, c := range fixtureMarker() {
		fmt.Fprintf(&entities, "&#%d;", c)
		fmt.Fprintf(&unicode, "\\u%04x", c)
	}
	for _, format := range []string{"json", "xml"} {
		t.Run(format, func(t *testing.T) {
			var content string
			if format == "json" {
				content = `{"value":"` + unicode.String() + `"}`
			} else {
				content = `<failure>` + entities.String() + `</failure>`
			}
			safe, err := s.Redact(append([]byte{0xef, 0xbb, 0xbf}, []byte(content)...))
			require.NoError(t, err)
			if format == "json" {
				var value map[string]string
				require.NoError(t, json.Unmarshal(bytes.TrimPrefix(safe, []byte{0xef, 0xbb, 0xbf}), &value))
				assertNoFragments(t, []byte(value["value"]), fixtureMarker())
			} else {
				var value string
				require.NoError(t, xml.Unmarshal(safe, &value))
				assertNoFragments(t, []byte(value), fixtureMarker())
			}
		})
	}
}
