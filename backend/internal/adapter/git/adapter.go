// Package gitadapter is the adapter git (TZ 5.5, 10.3): refs and the inventory of Markdown and OpenAPI
// documents of the default branch of any Git repository by ssh or https, from a bare partial clone kept in
// the cache of its container. Nothing of the repository runs: there is no checkout, blobs are never
// fetched, hooks, filters, submodules and protocols other than https and ssh are off, and global and
// system configuration of git is not read.
//
// The adapter reads neither merge requests nor rights. A document of such a repository is visible to whoever
// access allows the repository of a Git hosting with the same normalized remote in a connected GitLab or
// GitHub.com (TZ 13.5): Normalize gives that address, host/group/app for ssh://git@host/group/app.git,
// git@host:group/app.git and https://host/group/app alike, and the address of a hosting repository comes
// from http_url_to_repo and ssh_url_to_repo of a GitLab project or clone_url and ssh_url of a GitHub.com
// repository. Without such a repository the area is closed for everyone (NFR-05).
package gitadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_producer       = "adapter-git"
	_documentSchema = "document/1"
	_refSchema      = "git_ref/1"
)

// Config bounds the work of the adapter; zero values take the defaults of TZ 14.9.
type Config struct {
	// CacheDir holds the clones; it is emptied on start.
	CacheDir string
	// CacheBytes bounds all clones on disk, 2 GiB by default.
	CacheBytes int64
	// CloneBytes bounds one clone, 1 GiB by default.
	CloneBytes int64
	// CloneTimeout bounds one clone, 15 minutes by default.
	CloneTimeout time.Duration
	// CallTimeout bounds the git commands of one call, 25 s by default: the core waits 40 s.
	CallTimeout time.Duration
	// PrepareWait is how long a call waits for a clone it started before it answers preparing, 10 s by default.
	PrepareWait time.Duration
	// RetryAfter is when a call answered preparing comes back, 30 s by default.
	RetryAfter time.Duration
	// FailureHold is how long a failed clone answers its error before the next try, 5 minutes by default.
	FailureHold time.Duration
	// CAFile is the authority of sources by https in addition to the system ones.
	CAFile string
	// Git is the git binary, git from PATH by default.
	Git string
	// Dial maps a checked address of a remote to the one git connects to. Tests only.
	Dial func(netip.Addr) netip.Addr
	// Observe sees the arguments and the environment of every git command. Tests only.
	Observe func(args, env []string)
	Now     func() time.Time
}

func (c *Config) defaults() {
	set := func(d *time.Duration, v time.Duration) {
		if *d == 0 {
			*d = v
		}
	}
	if c.CacheBytes == 0 {
		c.CacheBytes = 2 << 30
	}
	if c.CloneBytes == 0 {
		c.CloneBytes = 1 << 30
	}
	set(&c.CloneTimeout, 15*time.Minute)
	set(&c.CallTimeout, 25*time.Second)
	set(&c.PrepareWait, 10*time.Second)
	set(&c.RetryAfter, 30*time.Second)
	set(&c.FailureHold, 5*time.Minute)
	if c.Git == "" {
		c.Git = "git"
	}
	if c.Now == nil {
		c.Now = time.Now
	}
}

type Adapter struct {
	adaptersdk.Unimplemented
	cfg     Config
	client  *adaptersdk.SourceClient
	git     *runner
	clones  *clones
	local   *session
	literal *session
	version string
}

// New checks the git binary and empties the cache directory.
func New(cfg Config, client *adaptersdk.SourceClient) (*Adapter, error) {
	cfg.defaults()
	if cfg.CacheDir == "" {
		return nil, errors.New("gitadapter: cache directory is not set")
	}
	cache, err := newClones(filepath.Join(cfg.CacheDir, "clones"), cfg.CacheBytes, cfg.CloneBytes, cfg.CloneTimeout, cfg.FailureHold, cfg.Now)
	if err != nil {
		return nil, err
	}
	work := filepath.Join(cfg.CacheDir, "work")
	if err := os.RemoveAll(work); err != nil {
		return nil, fmt.Errorf("gitadapter: clear work directory: %w", err)
	}
	if err := os.MkdirAll(work, 0o700); err != nil {
		return nil, fmt.Errorf("gitadapter: work directory: %w", err)
	}
	a := &Adapter{
		cfg: cfg, client: client, clones: cache,
		git:     &runner{git: cfg.Git, caFile: cfg.CAFile, work: work, dial: cfg.Dial, observe: cfg.Observe},
		local:   &session{dir: work, env: baseEnv(work)},
		literal: &session{dir: work, env: append(baseEnv(work), "GIT_LITERAL_PATHSPECS=1")},
	}
	out, err := a.git.output(context.Background(), a.local, "", 4096, "version")
	if err != nil {
		return nil, fmt.Errorf("gitadapter: git is not available: %w", err)
	}
	a.version = strings.TrimPrefix(strings.TrimSpace(string(out)), "git version ")
	if len(a.version) > 64 {
		a.version = a.version[:64]
	}
	return a, nil
}

// Close stops the clones in progress.
func (a *Adapter) Close() error {
	a.clones.Close()
	return nil
}

// CacheUsage is the size of the clones on disk.
func (a *Adapter) CacheUsage() int64 { return a.clones.usage() }

func (a *Adapter) Probe(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	srv, err := ParseServer(in.BaseUrl)
	if err != nil {
		return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "base_url is not an https or ssh address of a server")
	}
	creds, scheme, err := readCredentials(ctx, call)
	if err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	if scheme != srv.Scheme {
		return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "credentials are for "+scheme+", base_url is "+srv.Scheme)
	}
	now := a.cfg.Now().UTC().Truncate(time.Second)
	scopes := make([]adaptersdk.ScopeCheck, 0, len(in.Scopes))
	verified := false
	for _, s := range in.Scopes {
		check := adaptersdk.ScopeCheck{Kind: adaptersdk.ScopeCheckKind(s.Kind), NativeScopeId: s.NativeScopeId, State: adaptersdk.ScopeCheckStateMissing}
		reason, err := a.probeScope(ctx, srv, creds, s)
		switch {
		case err != nil:
			return adaptersdk.ProbeResult{}, err
		case reason != "":
			check.ReasonCode = &reason
		default:
			name := s.NativeScopeId[strings.Index(s.NativeScopeId, "/")+1:]
			check.State, check.DisplayName, verified = adaptersdk.ScopeCheckStateVerified, &name, true
		}
		scopes = append(scopes, check)
	}
	current := adaptersdk.Capability{Capability: adaptersdk.CapabilityCapabilityReadCurrent, State: adaptersdk.CapabilityStateVerified, CheckedAt: now}
	if !verified {
		reason := "no_repository"
		current.State, current.ReasonCode = adaptersdk.CapabilityStateMissing, &reason
	}
	version := a.version
	return adaptersdk.ProbeResult{
		ContractVersions: []string{adaptersdk.CurrentContractVersion},
		Provider:         adaptersdk.ProbeResultProviderGit,
		InstanceKey:      srv.Key(),
		Version:          &version,
		Capabilities:     []adaptersdk.Capability{current},
		Scopes:           scopes,
	}, nil
}

// probeScope reads the default branch of a repository without a clone; a refusal of the credentials is an
// error of the probe, every other failure is the reason of the scope.
func (a *Adapter) probeScope(ctx context.Context, srv Server, creds credentials, s adaptersdk.ScopeRef) (string, error) {
	if s.Kind != adaptersdk.ScopeRefKindRepository {
		return "not_supported", nil
	}
	rem, err := RemoteOf(srv, s.NativeScopeId)
	if err != nil {
		return "not_normalized", nil
	}
	ctx, cancel := context.WithTimeout(ctx, a.cfg.CallTimeout)
	defer cancel()
	branch, sha, err := a.lsRemote(ctx, rem, creds)
	var e *adaptersdk.Error
	switch {
	case errors.As(err, &e) && e.Code == adaptersdk.AdapterErrorCodeAuth:
		return "", err
	case errors.As(err, &e):
		return string(e.Code), nil
	case err != nil:
		return "", err
	}
	adaptersdk.Call{}.Log(ctx).Info().Str("repository", rem.ID).Str("default_branch", branch).Str("head", sha).Msg("repository probed")
	return "", nil
}

// lsRemote returns the default branch of a remote and its commit.
func (a *Adapter) lsRemote(ctx context.Context, rem Remote, creds credentials) (branch, sha string, err error) {
	s, err := a.session(ctx, rem, creds)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = s.Close() }()
	out, err := a.git.output(ctx, s, "", 1<<20, "ls-remote", "--symref", "--", s.url, "HEAD")
	if err != nil {
		return "", "", err
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if ref, ok := strings.CutPrefix(line, "ref: "); ok {
			branch, _, _ = strings.Cut(ref, "\t")
		} else if id, name, ok := strings.Cut(line, "\t"); ok && name == "HEAD" {
			sha = id
		}
	}
	if !strings.HasPrefix(branch, "refs/heads/") || !_sha.MatchString(sha) {
		return "", "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "repository has no default branch")
	}
	return branch, sha, nil
}

// session checks the host of the remote by the network scope of the installation before git starts and pins
// git to the checked address (TZ 13.7).
func (a *Adapter) session(ctx context.Context, rem Remote, creds credentials) (*session, error) {
	addrs, err := a.client.CheckHost(ctx, rem.Host)
	if err != nil {
		return nil, err
	}
	return a.git.open(rem, creds, addrs[0])
}

// credentials reads the form fields of the connection; the token means https, the key means ssh.
func readCredentials(ctx context.Context, call adaptersdk.Call) (credentials, string, error) {
	raw, err := call.Credential(ctx, adaptersdk.CredentialsInputPurposeCollect)
	if err != nil {
		return credentials{}, "", err
	}
	var fields credentials
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return credentials{}, "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "credentials are not a JSON object of the connection form")
	}
	scheme := "https"
	switch {
	case fields.Token != "" && fields.PrivateKey != "":
		return credentials{}, "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "credentials have both token and private_key")
	case fields.PrivateKey != "":
		scheme = "ssh"
	}
	c, err := parseCredentials(raw, scheme)
	return c, scheme, err
}

// target is the repository of a scope; the scheme comes from the credentials, the host from the scope.
func target(ctx context.Context, call adaptersdk.Call, scope adaptersdk.ScopeRef) (Remote, credentials, error) {
	creds, scheme, err := readCredentials(ctx, call)
	if err != nil {
		return Remote{}, credentials{}, err
	}
	notFound := adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "scope is not a normalized repository address")
	if scope.Kind != adaptersdk.ScopeRefKindRepository {
		return Remote{}, credentials{}, notFound
	}
	key, _, _ := strings.Cut(scope.NativeScopeId, "/")
	srv, err := ParseServer(scheme + "://" + key)
	if err != nil {
		return Remote{}, credentials{}, notFound
	}
	rem, err := RemoteOf(srv, scope.NativeScopeId)
	if err != nil {
		return Remote{}, credentials{}, notFound
	}
	return rem, creds, nil
}

// acquire returns the clone of the repository of a connection, starting it when there is none.
func (a *Adapter) acquire(ctx context.Context, call adaptersdk.Call, rem Remote, creds credentials) (*clone, error) {
	sum := sha256.Sum256([]byte(call.Organization.String() + "/" + call.Connection.String() + "/" + rem.ID))
	key := hex.EncodeToString(sum[:16])
	return a.clones.acquire(ctx, key, a.cfg.PrepareWait, a.cfg.RetryAfter, func(ctx context.Context, dir string) error {
		s, err := a.session(ctx, rem, creds)
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		return a.git.run(ctx, s, "", nil, "clone", "--bare", "--filter=blob:none", "--template=", "--quiet", "--", s.url, dir)
	})
}

// update fetches the branches and tags of the remote into a ready clone and follows its default branch.
func (a *Adapter) update(ctx context.Context, cl *clone, rem Remote, creds credentials) error {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	defer a.clones.resized(cl)
	s, err := a.session(ctx, rem, creds)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	if err := a.git.run(ctx, a.local, cl.dir, nil, "remote", "set-url", "origin", s.url); err != nil {
		return err
	}
	if err := a.git.run(ctx, s, cl.dir, nil, "fetch", "--prune", "--quiet", "origin",
		"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"); err != nil {
		return err
	}
	branch, _, err := a.lsRemote(ctx, rem, creds)
	if err != nil {
		return err
	}
	return a.git.run(ctx, a.local, cl.dir, nil, "symbolic-ref", "HEAD", branch)
}

// cursor is the position in a selection: the base commit the documents are diffed from (empty for all of
// them), the head commit and the number of items already given. native_id carries it as base..head/offset.
type cursor struct {
	base, head string
	offset     int
}

func (c cursor) String() string { return c.base + ".." + c.head + "/" + strconv.Itoa(c.offset) }

func parseCursor(raw string) (cursor, bool) {
	span, off, ok := strings.Cut(raw, "/")
	base, head, ok2 := strings.Cut(span, "..")
	n, err := strconv.Atoi(off)
	if !ok || !ok2 || err != nil || n < 0 || !_sha.MatchString(head) || (base != "" && !_sha.MatchString(base)) {
		return cursor{}, false
	}
	return cursor{base: base, head: head, offset: n}, true
}

// item is one record of a selection.
type item struct {
	ref     *gitRef
	doc     *document
	deleted string
}

func (a *Adapter) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	if in.SchemaId != adaptersdk.FetchInputSchemaIdDocuments {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "the adapter git serves documents only")
	}
	rem, creds, err := target(ctx, call, in.Scope)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.cfg.CallTimeout)
	defer cancel()
	cl, err := a.acquire(ctx, call, rem, creds)
	if err != nil {
		return nil, err
	}
	defer a.clones.release(cl)

	cur, headTime, err := a.position(ctx, cl, rem, creds, in)
	if err != nil {
		return nil, err
	}
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	items, err := a.items(ctx, cl.dir, cur, in.ObjectKeys, rem)
	if err != nil {
		return nil, err
	}
	added, err := a.addRecords(ctx, page, cl.dir, rem, cur.head, items[min(cur.offset, len(items)):], in.Limit)
	if err != nil {
		return nil, err
	}
	sel := envelope.Selection{End: len(in.ObjectKeys) > 0 || cur.offset >= len(items), Truncated: []envelope.TruncatedHistory{}}
	total := len(items)
	sel.TotalCount = &total
	if in.Cursor != nil {
		sel.Cursor = &envelope.Cursor{ModifiedAt: in.Cursor.ModifiedAt, NativeId: in.Cursor.NativeId}
	}
	if !sel.End {
		next := cur
		next.offset += added
		sel.NextCursor = &envelope.Cursor{ModifiedAt: headTime.UTC(), NativeId: next.String()}
	}
	return a.packageManifest(rem, cur.head, envelope.Period{From: in.Period.From, To: in.Period.To}, sel), nil
}

// position finds where a selection stands. A cursor of this adapter continues its selection on the commits
// it names. Otherwise the clone is fetched first and the documents are diffed from the commit the cursor
// names, or from the last commit before its time; without a cursor every document is read. The last page of
// a selection is empty, so the cursor sync keeps after it carries the time of head.
func (a *Adapter) position(ctx context.Context, cl *clone, rem Remote, creds credentials, in adaptersdk.FetchInput) (cursor, time.Time, error) {
	if in.Cursor != nil && len(in.ObjectKeys) == 0 {
		if c, ok := parseCursor(in.Cursor.NativeId); ok {
			cl.mu.RLock()
			defer cl.mu.RUnlock()
			if !a.known(ctx, cl.dir, c.head) {
				return cursor{}, time.Time{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "cursor names a commit the clone has not")
			}
			info, err := a.commit(ctx, cl.dir, c.head)
			return c, info.at, err
		}
	}
	if err := a.update(ctx, cl, rem, creds); err != nil {
		return cursor{}, time.Time{}, err
	}
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	head, err := a.head(ctx, cl.dir)
	if err != nil {
		return cursor{}, time.Time{}, err
	}
	c := cursor{head: head.sha}
	switch {
	case in.Cursor == nil || len(in.ObjectKeys) > 0:
	case a.known(ctx, cl.dir, in.Cursor.NativeId):
		c.base = in.Cursor.NativeId
	default:
		base, ok, err := a.baseAt(ctx, cl.dir, in.Cursor.ModifiedAt)
		if err != nil {
			return cursor{}, time.Time{}, err
		}
		if ok {
			c.base = base
		}
	}
	return c, head.at, nil
}

// items are the records of a selection in order: every ref, then the documents of head or the ones the diff
// from base touched, a document missing from head being deleted. Keys pick documents by path instead.
func (a *Adapter) items(ctx context.Context, dir string, c cursor, keys []adaptersdk.ObjectKey, rem Remote) ([]item, error) {
	docs, err := a.tree(ctx, dir, c.head)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]*document, len(docs))
	for i := range docs {
		byPath[docs[i].path] = &docs[i]
	}
	var items []item
	pick := func(paths []string) {
		for _, p := range paths {
			if d, ok := byPath[p]; ok {
				items = append(items, item{doc: d})
			} else {
				items = append(items, item{deleted: p})
			}
		}
	}
	if len(keys) > 0 {
		var paths []string
		for _, k := range keys {
			if _, ok := docFormat(k.NativeId); k.ObjectKind == adaptersdk.ObjectKeyObjectKindDocument && k.NativeNs == rem.ID && ok && validDocPath(k.NativeId) {
				paths = append(paths, k.NativeId)
			}
		}
		pick(paths)
		return items, nil
	}
	refs, err := a.refs(ctx, dir)
	if err != nil {
		return nil, err
	}
	for i := range refs {
		items = append(items, item{ref: &refs[i]})
	}
	if c.base == "" {
		for i := range docs {
			items = append(items, item{doc: &docs[i]})
		}
		return items, nil
	}
	paths, err := a.changed(ctx, dir, c.base, c.head)
	if err != nil {
		return nil, err
	}
	pick(paths)
	return items, nil
}

// addRecords adds up to limit items to the page and returns how many fit.
func (a *Adapter) addRecords(ctx context.Context, page *adaptersdk.Page, dir string, rem Remote, head string, items []item, limit int) (int, error) {
	items = items[:min(len(items), max(limit, 1))]
	var paths []string
	for _, it := range items {
		switch {
		case it.doc != nil:
			paths = append(paths, it.doc.path)
		case it.deleted != "":
			paths = append(paths, it.deleted)
		}
	}
	last, err := a.lastCommits(ctx, dir, head, paths)
	if err != nil {
		return 0, err
	}
	for i, it := range items {
		rec, ok, err := record(rem, it, last)
		if err != nil {
			return i, err
		}
		if !ok {
			continue
		}
		if err := page.AddRecord(rec); err != nil {
			if errors.Is(err, adaptersdk.ErrPageFull) && i > 0 {
				return i, nil
			}
			return i, err
		}
	}
	return len(items), nil
}

func record(rem Remote, it item, last map[string]commitInfo) (*envelope.Record, bool, error) {
	if it.ref != nil {
		body, err := json.Marshal(envelope.GitRefV1{
			RefKind: it.ref.kind, Name: it.ref.short, CommitSha: it.ref.commit, DefaultBranch: it.ref.isDefault,
		})
		if err != nil {
			return nil, false, fmt.Errorf("gitadapter: encode ref: %w", err)
		}
		rev := it.ref.commit
		return &envelope.Record{
			RecordClass: envelope.RecordRecordClassSnapshot, RecordKind: envelope.RecordRecordKindGitRef, PayloadSchema: _refSchema,
			NativeNs: rem.ID, NativeId: it.ref.full, SourceRevision: &rev, Payload: body,
		}, true, nil
	}
	p := it.deleted
	if it.doc != nil {
		p = it.doc.path
	}
	commit, ok := last[p]
	if !ok {
		// A path the history of the default branch never touched is not a document of it.
		return nil, false, nil
	}
	at := commit.at
	body := envelope.DocumentV1{
		ContainerNativeId: rem.ID, Path: &p, AclState: envelope.DocumentV1AclStateKnown, UpdatedAt: &at, CommitSha: &commit.sha,
	}
	if it.doc != nil {
		body.DocFormat, body.VersionRef = it.doc.format, it.doc.blob
	} else {
		format, _ := docFormat(p)
		deleted := true
		body.DocFormat, body.VersionRef, body.Deleted = format, commit.sha, &deleted
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, false, fmt.Errorf("gitadapter: encode document: %w", err)
	}
	rev := body.VersionRef
	return &envelope.Record{
		RecordClass: envelope.RecordRecordClassObject, RecordKind: envelope.RecordRecordKindDocument, PayloadSchema: _documentSchema,
		NativeNs: rem.ID, NativeId: p, SourceRevision: &rev, Payload: raw,
	}, true, nil
}

func (a *Adapter) packageManifest(rem Remote, head string, period envelope.Period, sel envelope.Selection) *envelope.Manifest {
	version := a.version
	readID := head
	return &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaIdDocuments, SchemaVersion: "1",
		Source:       envelope.Source{Provider: envelope.SourceProviderGit, InstanceKey: rem.Key(), Version: &version},
		Area:         envelope.AreaRef{Kind: envelope.AreaRefKindRepository, NativeId: rem.ID},
		Completeness: envelope.ManifestCompletenessComplete,
		Period:       period,
		Selection:    sel,
		ReadId:       &readID,
		Producer:     envelope.Producer{Name: _producer, Version: a.version},
		CreatedAt:    a.cfg.Now().UTC(),
	}
}

// Manifest lists the documents of head of the default branch with their blobs; the cursor is head/offset.
func (a *Adapter) Manifest(ctx context.Context, call adaptersdk.Call, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	if in.SchemaId != adaptersdk.ManifestInputSchemaIdDocuments {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "the adapter git serves documents only")
	}
	rem, creds, err := target(ctx, call, in.Scope)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.cfg.CallTimeout)
	defer cancel()
	cl, err := a.acquire(ctx, call, rem, creds)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	defer a.clones.release(cl)
	head, offset := "", 0
	if in.Cursor != nil {
		h, off, ok := strings.Cut(*in.Cursor, "/")
		n, perr := strconv.Atoi(off)
		if !ok || perr != nil || n < 0 || !_sha.MatchString(h) {
			return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "cursor is not a manifest cursor of this adapter")
		}
		head, offset = h, n
	} else if err := a.update(ctx, cl, rem, creds); err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	if head == "" {
		info, err := a.head(ctx, cl.dir)
		if err != nil {
			return adaptersdk.ManifestPage{}, err
		}
		head = info.sha
	}
	docs, err := a.tree(ctx, cl.dir, head)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	end := min(len(docs), offset+max(in.Limit, 1))
	res := adaptersdk.ManifestPage{Items: make([]adaptersdk.ManifestItem, 0, max(end-offset, 0))}
	for _, d := range docs[min(offset, end):end] {
		version := d.blob
		res.Items = append(res.Items, adaptersdk.ManifestItem{RecordKind: "document", NativeNs: rem.ID, NativeId: d.path, Version: &version})
	}
	total := len(docs)
	res.TotalCount = &total
	if end < len(docs) {
		next := head + "/" + strconv.Itoa(end)
		res.NextCursor = &next
	}
	return res, nil
}
