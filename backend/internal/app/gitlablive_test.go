package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func TestGitLabDiffLiveTwoHeads(t *testing.T) {
	host, token := os.Getenv("CP202_GITLAB_URL"), os.Getenv("CP202_GITLAB_TOKEN")
	if host == "" || token == "" {
		t.Skip("disposable GitLab required")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	call := func(method, path string, body any, out any) int {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), method, host+"/api/v4/"+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("PRIVATE-TOKEN", token)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		if res.StatusCode < 300 && out != nil {
			require.NoError(t, json.NewDecoder(res.Body).Decode(out))
		} else {
			_, _ = io.Copy(io.Discard, res.Body)
		}
		return res.StatusCode
	}
	var project struct {
		ID     int    `json:"id"`
		Branch string `json:"default_branch"`
	}
	require.Equal(t, 201, call("POST", "projects", map[string]any{"name": "cp202-disposable-fixture", "initialize_with_readme": true, "visibility": "private"}, &project))
	require.Positive(t, project.ID)
	root := "projects/" + strconv.Itoa(project.ID)
	committed := func(method, branch, contents string) {
		t.Helper()
		expected := 200
		if method == http.MethodPost {
			expected = 201
		}
		require.Equal(t, expected, call(method, root+"/repository/files/diff-fixture.txt", map[string]string{
			"branch": branch, "content": contents, "commit_message": "test fixture",
		}, nil))
	}
	committed("POST", project.Branch, "alpha\nold\nomega\n")
	require.Equal(t, 201, call("POST", root+"/repository/branches", map[string]string{"branch": "topic", "ref": project.Branch}, nil))
	committed("PUT", "topic", "alpha\nnew fixture text\nomega\n")
	var mr struct {
		ID  int `json:"id"`
		IID int `json:"iid"`
	}
	require.Equal(t, 201, call("POST", root+"/merge_requests", map[string]string{"source_branch": "topic", "target_branch": project.Branch, "title": "Test diff versions"}, &mr))
	mrPath := fmt.Sprintf("%s/merge_requests/%d", root, mr.IID)
	wait := func(previous string) (string, string) {
		t.Helper()
		for range 65 {
			var versions []struct {
				Head string `json:"head_commit_sha"`
				Base string `json:"base_commit_sha"`
			}
			if call("GET", mrPath+"/versions", nil, &versions) == 200 && len(versions) != 0 &&
				versions[0].Head != "" && versions[0].Head != previous && versions[0].Base != "" {
				return versions[0].Head, versions[0].Base
			}
			select {
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			case <-time.After(time.Second):
			}
		}
		t.Fatal("GitLab did not prepare a matching diff version")
		return "", ""
	}
	target, err := url.Parse(host)
	require.NoError(t, err)
	source := httputil.NewSingleHostReverseProxy(target)
	direct := source.Director
	source.Director = func(req *http.Request) {
		direct(req)
		req.Host = target.Host
	}
	srv := newGitLabTestEnvWithSource(t, source)
	raw, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)
	srv.target.Secret = string(raw)
	srv.target.Probe.Scopes[0].NativeScopeId = strconv.Itoa(project.ID)
	srv.probe(t)
	in := srv.target.Fetch
	in.Scope.NativeScopeId = strconv.Itoa(project.ID)
	in.ObjectKeys = []adaptersdk.ObjectKey{{ObjectKind: adaptersdk.ObjectKeyObjectKindMergeRequest, NativeId: strconv.Itoa(mr.ID)}}
	e := newNormalizeEnv(t)
	read := func(head, base string, want []entity.MRDiffRange) entity.MRDiff {
		t.Helper()
		status, raw := srv.call(t, http.MethodPost, "fetch", in)
		require.Equal(t, http.StatusOK, status)
		receipt := e.imported(t, e.admin, raw)
		require.Equal(t, entity.ReceiptAccepted, receipt.Status)
		e.drain(t)
		area := mrArea(t, e, receipt, strconv.Itoa(project.ID))
		scope := e.areaScope(t, e.admin, area)
		id := mrID(t, e, strconv.Itoa(mr.ID))
		got, err := e.norm.MRDiffAt(t.Context(), scope, e.orgID, id, head, math.MaxInt64)
		require.NoError(t, err)
		assert.Equal(t, base, got.BaseSHA)
		assert.Equal(t, entity.MRDiffComplete, got.Completeness)
		require.Len(t, got.Files, 1)
		assert.Equal(t, want, got.Files[0].HeadRanges)
		assert.Equal(t, new(len(want)), got.Files[0].AddedLines)
		assert.Equal(t, new(1), got.Files[0].DeletedLines)
		return got
	}
	head, base := wait("")
	first := read(head, base, []entity.MRDiffRange{{From: 2, To: 2}})
	committed("PUT", "topic", "alpha\nnew fixture text\nomega\nextra fixture text\n")
	next, base := wait(head)
	second := read(next, base, []entity.MRDiffRange{{From: 2, To: 2}, {From: 4, To: 4}})
	assert.NotEqual(t, first.RevisionID, second.RevisionID)
	assert.Equal(t, 2, e.n(t, "SELECT count(*) FROM normalize.mr_diff"))
}
