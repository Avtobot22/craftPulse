package testadapter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_defaultHang       = 45 * time.Second
	_defaultAfterBytes = 64
	_accessMaxAge      = 60
)

var _jiraCapabilities = []string{
	"read_current", "read_status_history", "read_field_history", "read_resolution_history", "read_permissions",
}

var _historyCapabilities = []string{"read_status_history", "read_field_history", "read_resolution_history"}

// Adapter serves the recorded dataset under a scenario. A scenario file, if set, is read again when it
// changes, so access can be revoked while the adapter runs.
type Adapter struct {
	adaptersdk.Unimplemented
	data *dataset
	now  func() time.Time

	mu       sync.Mutex
	cfg      Config
	attempts map[string]int
	path     string
	modTime  time.Time
}

func New(cfg Config) (*Adapter, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("testadapter: scenario: %w", err)
	}
	data, err := loadDataset()
	if err != nil {
		return nil, err
	}
	return &Adapter{data: data, now: time.Now, cfg: cfg, attempts: map[string]int{}}, nil
}

// NewFromFile serves the scenario of path and follows its changes; an empty path is the default scenario.
func NewFromFile(path string) (*Adapter, error) {
	if path == "" {
		return New(Config{})
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("testadapter: %w", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	a, err := New(cfg)
	if err != nil {
		return nil, err
	}
	a.path, a.modTime = path, info.ModTime()
	return a, nil
}

// SetConfig switches the scenario for the next call. Attempt counters restart only if faults change.
func (a *Adapter) SetConfig(cfg Config) error {
	if err := cfg.validate(); err != nil {
		return fmt.Errorf("testadapter: scenario: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setLocked(cfg)
	return nil
}

func (a *Adapter) setLocked(cfg Config) {
	if !reflect.DeepEqual(a.cfg.Faults, cfg.Faults) {
		a.attempts = map[string]int{}
	}
	a.cfg = cfg
}

func (a *Adapter) reloadLocked() error {
	if a.path == "" {
		return nil
	}
	info, err := os.Stat(a.path)
	if err != nil || info.ModTime().Equal(a.modTime) {
		return err
	}
	cfg, err := LoadConfig(a.path)
	if err != nil {
		return err
	}
	a.modTime = info.ModTime()
	a.setLocked(cfg)
	return nil
}

// begin counts the attempt, picks the fault and writes the request journal line. The journal has the
// operation and its position only: the test adapter never reads credentials.
func (a *Adapter) begin(ctx context.Context, call adaptersdk.Call, op, schema string, page int) (Config, *Fault) {
	a.mu.Lock()
	reloadErr := a.reloadLocked()
	key := op + "|" + schema + "|" + strconv.Itoa(page)
	a.attempts[key]++
	attempt := a.attempts[key]
	cfg := a.cfg
	var fault *Fault
	for i := range cfg.Faults {
		if cfg.Faults[i].matches(op, schema, page, attempt) {
			fault = &cfg.Faults[i]
			break
		}
	}
	a.mu.Unlock()

	log := call.Log(ctx)
	if reloadErr != nil {
		log.Warn().Err(reloadErr).Msg("scenario is not reloaded, the previous one is kept")
	}
	event := log.Info().Str("operation", op).Str("schema_id", schema).Int("page", page).Int("attempt", attempt)
	if fault != nil {
		event = event.Str("fault", fault.Kind)
	}
	event.Msg("source call")
	return cfg, fault
}

// answer turns a fault into the outcome of the call; stream and schema faults are applied by the caller.
func (a *Adapter) answer(ctx context.Context, f *Fault) error {
	if f == nil {
		return nil
	}
	retryAt := a.now().Add(time.Duration(f.RetryAfterSeconds) * time.Second).UTC().Truncate(time.Second)
	switch f.Kind {
	case FaultAuth:
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "source returned 401")
	case FaultPermissionMissing:
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodePermissionMissing, "source returned 403")
	case FaultRateLimited:
		e := adaptersdk.NewError(adaptersdk.AdapterErrorCodeRateLimited, "source returned 429")
		if f.RetryAfterSeconds > 0 {
			e.RetryAt = &retryAt
		}
		return e
	case FaultUnavailable:
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "source returned 503")
	case FaultPreparing:
		return adaptersdk.Preparing(retryAt, "test adapter prepares the data")
	case FaultHang:
		delay := _defaultHang
		if f.DelayMS > 0 {
			delay = time.Duration(f.DelayMS) * time.Millisecond
		}
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "source did not answer in time")
		}
	}
	return nil
}

// violates reports whether the answer must break the schema of adapter.yaml or envelope.yaml.
func violates(f *Fault) bool { return f != nil && f.Kind == FaultSchemaViolation }

func (a *Adapter) Probe(ctx context.Context, call adaptersdk.Call, in adaptersdk.ProbeInput) (adaptersdk.ProbeResult, error) {
	cfg, fault := a.begin(ctx, call, OpProbe, "", 1)
	if err := a.answer(ctx, fault); err != nil {
		return adaptersdk.ProbeResult{}, err
	}
	if cfg.Edition == EditionUnsupported {
		return adaptersdk.ProbeResult{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnsupportedEdition,
			"edition_mismatch: unsupported Jira edition")
	}
	now := a.now().UTC().Truncate(time.Second)
	enabled := cfg.Capabilities
	if len(enabled) == 0 {
		enabled = _jiraCapabilities
	}
	capabilities := make([]adaptersdk.Capability, 0, len(_jiraCapabilities))
	for _, name := range _jiraCapabilities {
		c := adaptersdk.Capability{Capability: adaptersdk.CapabilityCapability(name), State: adaptersdk.CapabilityStateVerified, CheckedAt: now}
		if !slices.Contains(enabled, name) || (cfg.HistoryDenied && slices.Contains(_historyCapabilities, name)) {
			reason := "permission_missing"
			c.State, c.ReasonCode = adaptersdk.CapabilityStateMissing, &reason
		}
		capabilities = append(capabilities, c)
	}
	scopes := make([]adaptersdk.ScopeCheck, 0, len(in.Scopes))
	for _, s := range in.Scopes {
		check := adaptersdk.ScopeCheck{Kind: adaptersdk.ScopeCheckKind(s.Kind), NativeScopeId: s.NativeScopeId, State: adaptersdk.ScopeCheckStateVerified}
		if p, ok := a.data.hasProject(s.NativeScopeId); ok && s.Kind == adaptersdk.ScopeRefKindTrackerProject {
			check.DisplayName = &p.Name
		} else {
			reason := "not_found"
			check.State, check.ReasonCode = adaptersdk.ScopeCheckStateMissing, &reason
		}
		scopes = append(scopes, check)
	}
	edition, version, mode := EditionCloud, a.data.Version, adaptersdk.ProbeResultAccessModeServiceAccount
	res := adaptersdk.ProbeResult{
		ContractVersions: []string{adaptersdk.CurrentContractVersion},
		Provider:         adaptersdk.ProbeResultProviderJiraCloud,
		InstanceKey:      a.data.InstanceKey,
		Edition:          &edition,
		Version:          &version,
		AccessMode:       &mode,
		Capabilities:     capabilities,
		Scopes:           scopes,
	}
	if violates(fault) {
		res.InstanceKey = ""
	}
	return res, nil
}

func (a *Adapter) feed(cfg Config, schema string, scope adaptersdk.ScopeRef) ([]entry, error) {
	if _, ok := a.data.hasProject(scope.NativeScopeId); !ok || scope.Kind != adaptersdk.ScopeRefKindTrackerProject {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "project "+scope.NativeScopeId+" not found")
	}
	feed, ok := a.data.Feeds[schema]
	if !ok {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "schema_id "+schema+" is not served by the test adapter")
	}
	if cfg.HistoryDenied && schema == "task_events" {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodePermissionMissing, "source returned 403 to the changelog")
	}
	return feed, nil
}

func pageOf(position, limit int) int { return position/limit + 1 }

func (a *Adapter) Fetch(ctx context.Context, call adaptersdk.Call, in adaptersdk.FetchInput, page *adaptersdk.Page) (*envelope.Manifest, error) {
	schema := string(in.SchemaId)
	limit := max(in.Limit, 1)
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	feed, feedErr := a.feed(cfg, schema, in.Scope)
	feed = inPeriod(feed, in.Period.From, in.Period.To)
	if len(in.ObjectKeys) == 0 && schema == "tasks" {
		feed = cfg.paged(feed)
	}

	var selected []entry
	start := 0
	switch {
	case len(in.ObjectKeys) > 0:
		// A key of a task asks for the whole changelog of the task: that is how a cut history is read on.
		keys := make(map[[2]string]bool, len(in.ObjectKeys))
		owners := map[string]bool{}
		for _, k := range in.ObjectKeys {
			keys[[2]string{k.NativeNs, k.NativeId}] = true
			if k.ObjectKind == adaptersdk.ObjectKeyObjectKindTask && schema == "task_events" {
				owners[k.NativeId] = true
			}
		}
		for _, e := range latest(feed) {
			if keys[[2]string{e.NativeNS, e.NativeID}] || owners[e.NativeNS] {
				selected = append(selected, e)
			}
		}
	case in.Cursor != nil:
		start = after(feed, in.Cursor.ModifiedAt, in.Cursor.NativeId)
	}
	pageNo := pageOf(start, limit)
	_, fault := a.begin(ctx, call, OpFetch, schema, pageNo)
	if err := a.answer(ctx, fault); err != nil {
		return nil, err
	}
	if feedErr != nil {
		return nil, feedErr
	}
	if len(in.ObjectKeys) == 0 {
		selected = feed[start:min(start+limit, len(feed))]
	}
	end := len(in.ObjectKeys) > 0 || start+len(selected) >= len(feed)

	kind := _feedKinds[schema]
	truncated := []envelope.TruncatedHistory{}
	seen := map[string]bool{}
	for i, e := range selected {
		rec := &envelope.Record{
			RecordClass: envelope.RecordRecordClass(kind.class), RecordKind: envelope.RecordRecordKind(kind.kind),
			PayloadSchema: kind.schema, NativeNs: e.NativeNS, NativeId: e.NativeID, Payload: e.Payload,
		}
		if kind.class == "event" {
			at := e.ModifiedAt
			rec.EventTime = &at
		}
		if e.Revision != "" {
			rev := e.Revision
			rec.SourceRevision = &rev
		}
		if i == 0 && violates(fault) {
			rec.NativeId = ""
		}
		if err := page.AddRecord(rec); err != nil {
			return nil, err
		}
		owner := e.NativeID
		if kind.kind == "task_event" {
			owner = e.NativeNS
		}
		for _, t := range a.data.Truncated {
			if t.NativeID == owner && kind.kind == "task_event" && !seen[owner] && len(in.ObjectKeys) == 0 {
				seen[owner] = true
				from := t.HistoryFrom
				truncated = append(truncated, envelope.TruncatedHistory{
					RecordKind: envelope.TruncatedHistoryRecordKind(t.RecordKind), NativeNs: t.NativeNS, NativeId: t.NativeID, HistoryFrom: &from,
				})
			}
		}
	}

	selection := envelope.Selection{End: end, Truncated: truncated}
	total := len(feed)
	if len(in.ObjectKeys) == 0 {
		selection.TotalCount = &total
	}
	if in.Cursor != nil {
		selection.Cursor = &envelope.Cursor{ModifiedAt: in.Cursor.ModifiedAt, NativeId: in.Cursor.NativeId}
	}
	if !end {
		last := selected[len(selected)-1]
		selection.NextCursor = &envelope.Cursor{ModifiedAt: last.ModifiedAt, NativeId: last.NativeID}
	}
	if fault != nil && (fault.Kind == FaultDropConnection || fault.Kind == FaultTruncated) {
		page.WrapOutput(breaker(fault))
	}
	return a.envelopeManifest(schema, in, selection), nil
}

func (a *Adapter) envelopeManifest(schema string, in adaptersdk.FetchInput, selection envelope.Selection) *envelope.Manifest {
	edition, version := EditionCloud, a.data.Version
	readID := "adapter-test"
	return &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(schema), SchemaVersion: "1",
		Source:       envelope.Source{Provider: envelope.SourceProviderJiraCloud, InstanceKey: a.data.InstanceKey, Edition: &edition, Version: &version},
		Area:         envelope.AreaRef{Kind: envelope.AreaRefKindTrackerProject, NativeId: in.Scope.NativeScopeId},
		Completeness: envelope.ManifestCompletenessComplete,
		Period:       envelope.Period{From: in.Period.From, To: in.Period.To},
		Selection:    selection,
		ReadId:       &readID,
		Producer:     envelope.Producer{Name: "adapter-test", Version: "1"},
		CreatedAt:    a.data.RecordedAt,
	}
}

// breaker cuts the package after a number of bytes: truncated ends the response cleanly with a broken
// archive, drop_connection aborts the connection in the middle of the stream.
func breaker(f *Fault) func(io.Writer) io.Writer {
	limit := f.AfterBytes
	if limit == 0 {
		limit = _defaultAfterBytes
	}
	return func(w io.Writer) io.Writer {
		return &cutWriter{w: w, left: limit, drop: f.Kind == FaultDropConnection}
	}
}

type cutWriter struct {
	w    io.Writer
	left int
	drop bool
}

func (c *cutWriter) Write(p []byte) (int, error) {
	if c.left <= 0 {
		if c.drop {
			panic(http.ErrAbortHandler)
		}
		return len(p), nil
	}
	n := min(len(p), c.left)
	if _, err := c.w.Write(p[:n]); err != nil {
		return 0, err
	}
	c.left -= n
	if n < len(p) {
		if f, ok := c.w.(http.Flusher); ok {
			f.Flush()
		}
		if c.drop {
			panic(http.ErrAbortHandler)
		}
	}
	return len(p), nil
}

func (a *Adapter) Manifest(ctx context.Context, call adaptersdk.Call, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	schema := string(in.SchemaId)
	limit := max(in.Limit, 1)
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	feed, feedErr := a.feed(cfg, schema, in.Scope)
	objects := latest(inPeriod(feed, in.Period.From, in.Period.To))
	start := 0
	if in.Cursor != nil {
		n, err := strconv.Atoi(*in.Cursor)
		if err != nil || n < 0 || n > len(objects) {
			return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "cursor is not a cursor of this manifest")
		}
		start = n
	}
	_, fault := a.begin(ctx, call, OpManifest, schema, pageOf(start, limit))
	if err := a.answer(ctx, fault); err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	if feedErr != nil {
		return adaptersdk.ManifestPage{}, feedErr
	}
	stop := min(start+limit, len(objects))
	kind := _feedKinds[schema].kind
	items := make([]adaptersdk.ManifestItem, 0, stop-start)
	for _, e := range objects[start:stop] {
		at := e.ModifiedAt
		items = append(items, adaptersdk.ManifestItem{RecordKind: kind, NativeNs: e.NativeNS, NativeId: e.NativeID, UpdatedAt: &at})
	}
	if violates(fault) && len(items) > 0 {
		items[0].NativeId = ""
	}
	res := adaptersdk.ManifestPage{Items: items}
	total := len(objects)
	res.TotalCount = &total
	if stop < len(objects) {
		next := strconv.Itoa(stop)
		res.NextCursor = &next
	}
	return res, nil
}

func (a *Adapter) CheckAccess(ctx context.Context, call adaptersdk.Call, in adaptersdk.CheckAccessInput) (adaptersdk.CheckAccessResult, error) {
	cfg, fault := a.begin(ctx, call, OpCheckAccess, "", 1)
	if err := a.answer(ctx, fault); err != nil {
		return adaptersdk.CheckAccessResult{}, err
	}
	if cfg.Access.DelayMS > 0 {
		t := time.NewTimer(time.Duration(cfg.Access.DelayMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return adaptersdk.CheckAccessResult{}, ctx.Err()
		case <-t.C:
		}
	}
	now := a.now().UTC().Truncate(time.Second)
	user := in.UserId.String()
	decisions := make([]adaptersdk.AccessDecision, 0, len(in.Items))
	for _, item := range in.Items {
		decisions = append(decisions, decide(cfg.Access, user, item, now))
	}
	if violates(fault) && len(decisions) > 0 {
		decisions = decisions[:len(decisions)-1]
	}
	maxAge := _accessMaxAge
	return adaptersdk.CheckAccessResult{
		Mode: adaptersdk.CheckAccessResultModeServiceAccount, CheckedAt: now, MaxAgeSeconds: &maxAge, Decisions: decisions,
	}, nil
}

func decide(access Access, user string, item adaptersdk.AccessItem, now time.Time) adaptersdk.AccessDecision {
	for _, r := range access.Rules {
		if (r.UserID == "" || r.UserID == user) && (r.ItemKind == "" || r.ItemKind == string(item.ItemKind)) &&
			(r.NativeID == "" || r.NativeID == item.NativeId) {
			d := adaptersdk.AccessDecision{Decision: adaptersdk.AccessDecisionDecision(r.Decision), CheckedAt: now}
			if r.ReasonCode != "" {
				reason := r.ReasonCode
				d.ReasonCode = &reason
			}
			return d
		}
	}
	decision := access.Default
	if decision == "" {
		decision = "unknown"
	}
	return adaptersdk.AccessDecision{Decision: adaptersdk.AccessDecisionDecision(decision), CheckedAt: now}
}

var _ adaptersdk.Source = (*Adapter)(nil)
