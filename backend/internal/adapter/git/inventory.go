package gitadapter

import (
	"bytes"
	"context"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// _listLimit bounds the output of a listing of one commit; a tree beyond it is refused, not cut.
const _listLimit = 32 << 20

var (
	_sha     = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	errFound = errors.New("gitadapter: every path is found")
)

type gitRef struct {
	full, short string
	kind        envelope.GitRefV1RefKind
	commit      string
	isDefault   bool
}

type document struct {
	path   string
	blob   string
	format envelope.DocumentV1DocFormat
}

type commitInfo struct {
	sha string
	at  time.Time
}

// docFormat keeps Markdown and OpenAPI by name: the inventory reads trees, not file contents.
func docFormat(p string) (envelope.DocumentV1DocFormat, bool) {
	base := strings.ToLower(path.Base(p))
	switch ext := path.Ext(base); ext {
	case ".md", ".markdown":
		return envelope.DocumentV1DocFormatMarkdown, true
	case ".yaml", ".yml", ".json":
		stem := strings.TrimSuffix(base, ext)
		if strings.HasPrefix(stem, "openapi") || strings.HasPrefix(stem, "swagger") || strings.HasSuffix(stem, ".openapi") {
			return envelope.DocumentV1DocFormatOpenapi, true
		}
	}
	return "", false
}

// validDocPath refuses a path git could take for an option and one the envelope cannot carry.
func validDocPath(p string) bool {
	return p != "" && len(p) <= 1024 && !strings.HasPrefix(p, "-") && !strings.ContainsAny(p, "\x00\n") &&
		!slices.Contains(strings.Split(p, "/"), "..")
}

// refs lists branches and tags with the commit each one points to and marks the default branch.
func (a *Adapter) refs(ctx context.Context, dir string) ([]gitRef, error) {
	head, err := a.git.output(ctx, a.local, dir, 4096, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return nil, err
	}
	defaultBranch := strings.TrimSpace(string(head))
	out, err := a.git.output(ctx, a.local, dir, _listLimit, "for-each-ref",
		"--format=%(objectname)%00%(*objectname)%00%(refname)", "refs/heads", "refs/tags")
	if err != nil {
		return nil, err
	}
	var refs []gitRef
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "\x00")
		if len(parts) != 3 {
			continue
		}
		commit, peeled, full := parts[0], parts[1], parts[2]
		if peeled != "" {
			commit = peeled
		}
		r := gitRef{full: full, commit: commit, isDefault: full == defaultBranch}
		switch {
		case strings.HasPrefix(full, "refs/heads/"):
			r.short, r.kind = strings.TrimPrefix(full, "refs/heads/"), envelope.GitRefV1RefKindBranch
		case strings.HasPrefix(full, "refs/tags/"):
			r.short, r.kind = strings.TrimPrefix(full, "refs/tags/"), envelope.GitRefV1RefKindTag
		default:
			continue
		}
		if _sha.MatchString(commit) && r.short != "" && len(r.short) <= 255 && len(full) <= 1024 {
			refs = append(refs, r)
		}
	}
	return refs, nil
}

// head is the commit of the default branch and its time.
func (a *Adapter) head(ctx context.Context, dir string) (commitInfo, error) {
	return a.commit(ctx, dir, "HEAD")
}

func (a *Adapter) commit(ctx context.Context, dir, rev string) (commitInfo, error) {
	out, err := a.git.output(ctx, a.local, dir, 4096, "log", "-1", "--format=%H%x00%cI", rev, "--")
	if err != nil {
		return commitInfo{}, err
	}
	sha, at, ok := strings.Cut(strings.TrimSpace(string(out)), "\x00")
	t, perr := time.Parse(time.RFC3339, at)
	if !ok || perr != nil || !_sha.MatchString(sha) {
		return commitInfo{}, adaptersdk.InvalidResponseError("default branch has no commit")
	}
	return commitInfo{sha: sha, at: t}, nil
}

// baseAt is the last commit of the default branch at the time, or false before its first commit.
func (a *Adapter) baseAt(ctx context.Context, dir string, at time.Time) (sha string, found bool, err error) {
	out, err := a.git.output(ctx, a.local, dir, 4096, "rev-list", "-1", "--first-parent",
		"--before="+at.UTC().Format(time.RFC3339), "HEAD", "--")
	if err != nil {
		return "", false, err
	}
	sha = strings.TrimSpace(string(out))
	return sha, _sha.MatchString(sha), nil
}

// known reports whether the clone has the commit; a SHA of another history is not one.
func (a *Adapter) known(ctx context.Context, dir, sha string) bool {
	if !_sha.MatchString(sha) {
		return false
	}
	_, err := a.git.output(ctx, a.local, dir, 4096, "cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// tree lists the documents of a commit with their blobs; the blobs themselves are never fetched.
func (a *Adapter) tree(ctx context.Context, dir, commit string) ([]document, error) {
	out, err := a.git.output(ctx, a.local, dir, _listLimit, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	var docs []document
	for entry := range bytes.SplitSeq(out, []byte{0}) {
		meta, p, ok := strings.Cut(string(entry), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			continue
		}
		format, ok := docFormat(p)
		if ok && validDocPath(p) && _sha.MatchString(fields[2]) {
			docs = append(docs, document{path: p, blob: fields[2], format: format})
		}
	}
	slices.SortFunc(docs, func(x, y document) int { return strings.Compare(x.path, y.path) })
	return docs, nil
}

// changed are the document paths a diff of two commits adds, modifies or deletes; trees are compared by
// their object IDs, so no blob is needed.
func (a *Adapter) changed(ctx context.Context, dir, base, head string) ([]string, error) {
	out, err := a.git.output(ctx, a.local, dir, _listLimit, "diff-tree", "-r", "-z", "--no-renames", "--name-only", base, head, "--")
	if err != nil {
		return nil, err
	}
	var paths []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if _, ok := docFormat(p); ok && validDocPath(p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// lastCommits finds for each path the last commit of the default branch that changed it, a deletion
// included. It walks the history newest first and stops as soon as every path is found.
func (a *Adapter) lastCommits(ctx context.Context, dir, head string, paths []string) (map[string]commitInfo, error) {
	found := make(map[string]commitInfo, len(paths))
	if len(paths) == 0 {
		return found, nil
	}
	wanted := make(map[string]bool, len(paths))
	for _, p := range paths {
		wanted[p] = true
	}
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	parser := &historyParser{wanted: wanted, found: found, complete: func() { stop(errFound) }}
	args := append([]string{
		"log", "-z", "--first-parent", "--diff-merges=first-parent", "--no-renames", "--name-only",
		"--format=%x1e%H %cI", head, "--",
	}, paths...)
	err := a.git.run(ctx, a.literal, dir, parser, args...)
	if err != nil && !errors.Is(err, errFound) {
		return nil, err
	}
	parser.flush()
	return found, nil
}

// historyParser reads `git log -z --name-only --format=%x1e%H %cI` as it streams.
type historyParser struct {
	wanted   map[string]bool
	found    map[string]commitInfo
	complete func()
	rest     []byte
}

func (h *historyParser) Write(p []byte) (int, error) {
	h.rest = append(h.rest, p...)
	for {
		start := bytes.IndexByte(h.rest, 0x1e)
		if start < 0 {
			return len(p), nil
		}
		next := bytes.IndexByte(h.rest[start+1:], 0x1e)
		if next < 0 {
			h.rest = h.rest[start:]
			return len(p), nil
		}
		h.record(h.rest[start+1 : start+1+next])
		h.rest = h.rest[start+1+next:]
		if len(h.found) == len(h.wanted) {
			h.complete()
			return len(p), nil
		}
	}
}

func (h *historyParser) flush() {
	if len(h.rest) > 0 && h.rest[0] == 0x1e {
		h.record(h.rest[1:])
	}
	h.rest = nil
}

func (h *historyParser) record(rec []byte) {
	header, names, _ := bytes.Cut(rec, []byte{0})
	sha, at, ok := strings.Cut(string(header), " ")
	t, err := time.Parse(time.RFC3339, at)
	if !ok || err != nil {
		return
	}
	for name := range bytes.SplitSeq(bytes.TrimLeft(names, "\n"), []byte{0}) {
		n := string(name)
		if _, seen := h.found[n]; h.wanted[n] && !seen {
			h.found[n] = commitInfo{sha: sha, at: t}
		}
	}
}
