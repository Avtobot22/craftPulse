package app

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _glDiffPath = "/api/v4/projects/7/merge_requests/1/versions/302"

func TestGitLabDiff_VersionPinnedAndPatchTextDiscarded(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, state, size string
		extra             string
		status            int
		want              envelope.MrDiffV1Completeness
	}{
		{"collected", "collected", "4", "", http.StatusOK, envelope.MrDiffV1CompletenessComplete},
		{"global overflow", "overflow", "5", "", http.StatusOK, envelope.MrDiffV1CompletenessPartial},
		{"file omitted", "collected", "5", `,{"old_path":"huge","new_path":"huge","too_large":true,"diff":""}`, http.StatusOK, envelope.MrDiffV1CompletenessPartial},
		{"unavailable version detail", "", "", "", http.StatusNotFound, envelope.MrDiffV1CompletenessPartial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := &gitLabFixture{}
			e := newGitLabTestEnvWithSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != _glDiffPath {
					fixture.ServeHTTP(w, r)
					return
				}
				assert.Equal(t, _gitLabToken, r.Header.Get("PRIVATE-TOKEN"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if tt.status != http.StatusOK {
					return
				}
				_, _ = io.WriteString(w, `{"id":302,"merge_request_id":10,"head_commit_sha":"`+_glHead+`",
					"base_commit_sha":"`+_glBase+`","start_commit_sha":"`+_glBase+`","state":"`+tt.state+`","real_size":"`+tt.size+`","diffs":[
					{"old_path":"src/a.go","new_path":"src/a.go","diff":"@@ -1 +1,2 @@\n-old fixture\n+local patch sentinel\n+second added\n"},
					{"old_path":"old-name","new_path":"new-name","renamed_file":true,"diff":""},
					{"old_path":"image.bin","new_path":"image.bin","diff":"Binary files a/image.bin and b/image.bin differ\n"},
					{"old_path":"removed","new_path":"removed","deleted_file":true,"diff":"@@ -1 +0,0 @@\n-old fixture\n"}`+tt.extra+`]}`)
			}))
			e.probe(t)
			in := e.target.Fetch
			in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"}}
			pkg, result := e.fetch(t, in)
			var diff envelope.MrDiffV1
			var found int
			for _, r := range result.Records {
				if r.Kind != "mr_diff" {
					continue
				}
				found++
				assert.Equal(t, "302", r.NativeID)
				assert.Equal(t, "10", r.NativeNS)
				assert.Nil(t, r.SourceRevision)
				rec, ok, err := pkg.Record(r.No)
				require.NoError(t, err)
				require.True(t, ok)
				assert.NotContains(t, string(rec.Payload), "local patch sentinel")
				require.NoError(t, json.Unmarshal(rec.Payload, &diff))
			}
			require.Equal(t, 1, found)
			assert.Equal(t, _glHead, diff.HeadSha)
			assert.Equal(t, _glBase, diff.BaseSha)
			assert.Equal(t, tt.want, diff.Completeness)
			if tt.status == http.StatusNotFound {
				assert.Empty(t, diff.Files)
				return
			}
			files := map[string]envelope.MrDiffFile{}
			for _, f := range diff.Files {
				files[f.NewPath] = f
			}
			assert.Equal(t, new(2), files["src/a.go"].AddedLines)
			assert.Equal(t, new(1), files["src/a.go"].DeletedLines)
			assert.Equal(t, []envelope.MrDiffRange{{From: 1, To: 2}}, files["src/a.go"].HeadRanges)
			assert.True(t, files["new-name"].RenamedFile)
			assert.False(t, files["new-name"].NewFile)
			assert.False(t, files["new-name"].DeletedFile)
			assert.True(t, files["image.bin"].Binary)
			assert.Empty(t, files["image.bin"].HeadRanges)
			assert.Equal(t, new(0), files["image.bin"].AddedLines)
			assert.True(t, files["removed"].DeletedFile)
			assert.Empty(t, files["removed"].HeadRanges)
			if tt.extra != "" {
				assert.Contains(t, files, "huge")
				assert.Nil(t, files["huge"].AddedLines)
			}
		})
	}
}

func TestGitLabDiff_NormalizesTwoHeadsAndIgnoresReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const file = "example.txt"
	gitDiffCommand(t, dir, "init", "--quiet")
	base := diffCommit(t, dir, file, "alpha\nold\nomega\n")
	firstHead := diffCommit(t, dir, file, "alpha\nnew fixture text\nomega\n")
	secondHead := diffCommit(t, dir, file, "alpha\nnew fixture text\nomega\nextra fixture text\n")

	e := newNormalizeEnv(t)
	var current atomic.Int32
	var omitFile atomic.Bool
	patches := []string{
		gitDiffCommand(t, dir, "diff", base+"..."+firstHead, "--", file),
		gitDiffCommand(t, dir, "diff", base+"..."+secondHead, "--", file),
	}
	numstat := []string{
		gitDiffCommand(t, dir, "diff", "--numstat", base+"..."+firstHead),
		gitDiffCommand(t, dir, "diff", "--numstat", base+"..."+secondHead),
	}
	heads := []string{firstHead, secondHead}
	fixture := &gitLabFixture{}
	srv := newGitLabTestEnvWithSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const root = "/api/v4/projects/7/merge_requests/1"
		i := int(current.Load())
		version := 302 + i
		meta := map[string]any{
			"id": version, "merge_request_id": 10, "head_commit_sha": heads[i], "base_commit_sha": base,
			"start_commit_sha": base, "created_at": []string{_glTime, _glNew}[i],
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case root:
			raw := strings.NewReplacer(_glBase, base, _glHead, heads[i]).Replace(glMR("7", "1", "10", []string{_glTime, _glNew}[i]))
			_, _ = io.WriteString(w, raw)
		case root + "/versions":
			_ = json.NewEncoder(w).Encode([]any{meta})
		case root + "/versions/" + strconv.Itoa(version):
			meta["state"], meta["real_size"] = "collected", "1"
			meta["diffs"] = []any{map[string]any{"old_path": file, "new_path": file, "diff": patches[i]}}
			if omitFile.Load() {
				meta["state"], meta["diffs"] = "overflow", []any{}
			}
			_ = json.NewEncoder(w).Encode(meta)
		default:
			fixture.ServeHTTP(w, r)
		}
	}))
	srv.probe(t)
	in := srv.target.Fetch
	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: "10"}}
	appendHead := func() entity.Receipt {
		status, raw := srv.call(t, http.MethodPost, "fetch", in)
		require.Equal(t, http.StatusOK, status)
		rc := e.imported(t, e.admin, raw)
		e.drain(t)
		return rc
	}

	omitFile.Store(true)
	incomplete := appendHead()
	require.Equal(t, entity.ReceiptAccepted, incomplete.Status)
	area := mrArea(t, e, incomplete, "7")
	scope := e.areaScope(t, e.admin, area)
	mr := mrID(t, e, "10")
	partial, err := e.norm.MRDiffAt(t.Context(), scope, e.orgID, mr, firstHead, math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, entity.MRDiffPartial, partial.Completeness)
	assert.Empty(t, partial.Files)
	omitFile.Store(false)
	require.Equal(t, entity.ReceiptAccepted, appendHead().Status)
	old, err := e.norm.MRDiffAt(t.Context(), scope, e.orgID, mr, firstHead, math.MaxInt64)
	require.NoError(t, err)
	seenEarlier, err := e.norm.MRDiffAt(t.Context(), scope, e.orgID, mr, firstHead, partial.KnownSeq)
	require.NoError(t, err)
	assert.Equal(t, partial, seenEarlier)
	current.Store(1)
	second := appendHead()
	require.Equal(t, entity.ReceiptAccepted, second.Status)
	newHead, err := e.norm.MRDiffAt(t.Context(), scope, e.orgID, mr, secondHead, math.MaxInt64)
	require.NoError(t, err)
	assert.NotEqual(t, old.RevisionID, newHead.RevisionID)
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.mr_diff"))
	for i, d := range []entity.MRDiff{old, newHead} {
		assert.Equal(t, entity.MRDiffComplete, d.Completeness)
		assert.Equal(t, base, d.BaseSHA)
		assert.Equal(t, heads[i], d.HeadSHA)
		require.Len(t, d.Files, 1)
		got := d.Files[0]
		require.NotNil(t, got.AddedLines)
		require.NotNil(t, got.DeletedLines)
		assert.Equal(t, strings.TrimSpace(numstat[i]), fmt.Sprintf("%d\t%d\t%s", *got.AddedLines, *got.DeletedLines, got.NewPath))
	}
	changed, err := e.norm.MRChangedLinesAt(t.Context(), scope, e.orgID, mr, secondHead, file, math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, []entity.MRDiffRange{{From: 2, To: 2}, {From: 4, To: 4}}, changed.File.HeadRanges)
	_, err = e.norm.MRDiffAt(t.Context(), scope, e.orgID, mr, secondHead, old.KnownSeq)
	assert.ErrorIs(t, err, entity.ErrNotFound)
	_, err = e.norm.MRDiffAt(t.Context(), entity.Scope{}, e.orgID, mr, secondHead, math.MaxInt64)
	assert.ErrorIs(t, err, entity.ErrNotFound)
	_, err = e.norm.MRDiffAt(t.Context(), scope, e.otherID, mr, secondHead, math.MaxInt64)
	assert.ErrorIs(t, err, entity.ErrNotFound)
	e.renormalize(t, second.ID)
	e.drain(t)
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.mr_diff"), "same diff after another normalization")
	assert.Zero(t, e.n(t, "SELECT count(*) FROM normalize.mr_diff WHERE files::text LIKE '%fixture text%'"))
}

func diffCommit(t *testing.T, dir, name, contents string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600))
	gitDiffCommand(t, dir, "add", name)
	gitDiffCommand(t, dir, "commit", "--quiet", "-m", "test diff")
	return strings.TrimSpace(gitDiffCommand(t, dir, "rev-parse", "HEAD"))
}

func gitDiffCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cfg := []string{"-C", dir, "-c", "user.email=test@example.test", "-c", "user.name=Fixture", "-c", "commit.gpgsign=false"}
	out, err := osexec.CommandContext(t.Context(), "git", append(cfg, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}
