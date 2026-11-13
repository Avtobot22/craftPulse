package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_maxName             = 200
	_maxURL              = 2048
	_maxAdapterKey       = 512
	_maxCredentialFields = 16
	_maxCredentialValue  = 8192
	_maxScopes           = 500
	_maxScopeID          = 255
	_maxPage             = 200
)

var _providers = []entity.Provider{
	entity.ProviderJiraCloud, entity.ProviderGitLab, entity.ProviderGitHub, entity.ProviderJenkins, entity.ProviderSonarQube,
	entity.ProviderConfluenceCloud, entity.ProviderGit, entity.ProviderClient,
}

var _scopeKinds = []entity.ScopeKind{
	entity.ScopeTrackerProject, entity.ScopeRepository, entity.ScopeWikiSpace, entity.ScopeCIProject, entity.ScopeAnalysisProject,
}

// Create stores a draft connection. The credentials go to the adapter before the row is written and are
// deleted there again if writing fails, so the core never holds them (TZ 13.6).
func (u *UseCase) Create(ctx context.Context, s entity.Subject, in entity.ConnectionInput) (entity.ConnectionState, error) {
	if err := u.validateCreate(ctx, in); err != nil {
		return entity.ConnectionState{}, err
	}
	id, err := u.d.NewID()
	if err != nil {
		return entity.ConnectionState{}, err
	}
	inst, err := u.provisionalInstance(ctx, s.OrganizationID, in.Provider, in.BaseURL)
	if err != nil {
		return entity.ConnectionState{}, err
	}
	now := u.d.Clock.Now()
	c := entity.Connection{
		ID: id, OrganizationID: s.OrganizationID, InstanceID: inst.ID, Provider: in.Provider, Name: in.Name,
		AdapterKind: in.AdapterKind, ContractVersion: ContractVersions[0], AccessMode: entity.AccessServiceAccount,
		Status: entity.ConnectionDraft, RowVersion: 1, CreatedBy: s.UserID, CreatedAt: now,
	}
	if in.AdapterKind == entity.AdapterClient {
		if err := u.sealKey(&c, in.AdapterURL, in.AdapterKey); err != nil {
			return entity.ConnectionState{}, err
		}
	}
	if len(in.Credentials) > 0 {
		if err := u.sendCredentials(ctx, c, in.Credentials); err != nil {
			return entity.ConnectionState{}, err
		}
		c.CredentialsSetAt = &now
	}
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		if err := u.d.Repo.CreateConnection(ctx, c); err != nil {
			return err
		}
		for _, sc := range in.Scopes {
			if _, err := u.d.Repo.AddScope(ctx, s.OrganizationID, c.ID, c.InstanceID, named(sc), now); err != nil {
				return err
			}
		}
		return u.audit(ctx, s, "createConnection", c.ID, "", "1")
	})
	if err != nil {
		if c.CredentialsSetAt != nil {
			u.dropCredentials(ctx, c)
		}
		return entity.ConnectionState{}, err
	}
	return u.State(ctx, s.OrganizationID, c.ID)
}

// List pages the connections of the organization, newest first.
func (u *UseCase) List(ctx context.Context, s entity.Subject, f entity.ConnectionFilter) (entity.ConnectionPage, error) {
	if f.Limit < 1 || f.Limit > _maxPage {
		return entity.ConnectionPage{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "limit", Code: entity.FieldOutOfRange}}}
	}
	limit := f.Limit
	f.Limit++
	var out entity.ConnectionPage
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		items, err := u.d.Repo.PageConnections(ctx, s.OrganizationID, f)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			last := items[limit-1]
			out.Next = &entity.AuditKey{OccurredAt: last.CreatedAt, ID: last.ID}
		}
		ids := make([]entity.ID, len(items))
		for i := range items {
			ids[i] = items[i].ID
		}
		scopes, err := u.d.Repo.ScopesOf(ctx, s.OrganizationID, ids)
		if err != nil {
			return err
		}
		caps, err := u.d.Repo.CapabilitiesOf(ctx, s.OrganizationID, ids)
		if err != nil {
			return err
		}
		out.Items = make([]entity.ConnectionState, 0, len(items))
		for i := range items {
			c := &items[i]
			inst, err := u.d.Repo.Instance(ctx, s.OrganizationID, c.InstanceID)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, entity.ConnectionState{Connection: *c, Instance: inst, Scopes: scopes[c.ID], Capabilities: caps[c.ID]})
		}
		return nil
	})
	return out, err
}

// Update applies a merge patch on the version the form read. A new address, adapter or scope set returns
// the connection to draft until a new probe; new credentials need a new probe as well, because every
// change of them, of the address, of the adapter or of the scopes moves row_version (TZ 12.5.3).
func (u *UseCase) Update(ctx context.Context, s entity.Subject, id entity.ID, baseVersion int, p entity.ConnectionPatch) (entity.ConnectionState, error) {
	cur, err := u.connection(ctx, s.OrganizationID, id)
	if err != nil {
		return entity.ConnectionState{}, err
	}
	if cur.Status == entity.ConnectionDeleted {
		return entity.ConnectionState{}, entity.ErrStateConflict
	}
	if cur.RowVersion != baseVersion {
		return entity.ConnectionState{}, entity.ErrVersionConflict
	}
	if err := u.validatePatch(ctx, cur, p); err != nil {
		return entity.ConnectionState{}, err
	}
	next := cur
	if p.AdapterURL != nil || p.AdapterKey != nil {
		rawURL, key := cur.AdapterURL, entity.Secret("")
		if p.AdapterURL != nil {
			rawURL = *p.AdapterURL
		}
		if p.AdapterKey != nil {
			key = *p.AdapterKey
		} else if key, err = u.adapterKey(cur); err != nil {
			return entity.ConnectionState{}, err
		}
		if err := u.sealKey(&next, rawURL, key); err != nil {
			return entity.ConnectionState{}, err
		}
	}
	now := u.d.Clock.Now()
	if len(p.Credentials) > 0 {
		if err := u.sendCredentials(ctx, next, p.Credentials); err != nil {
			return entity.ConnectionState{}, err
		}
		next.CredentialsSetAt = &now
	}
	err = u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		locked, err := u.d.Repo.LockConnection(ctx, s.OrganizationID, id)
		if err != nil {
			return err
		}
		if locked.RowVersion != baseVersion || locked.Status == entity.ConnectionDeleted {
			return entity.ErrVersionConflict
		}
		next.PausedUntil, next.AccessLostAt, next.Status = locked.PausedUntil, locked.AccessLostAt, locked.Status
		return u.applyPatch(ctx, s, cur, next, p, now)
	})
	if err != nil {
		return entity.ConnectionState{}, err
	}
	return u.State(ctx, s.OrganizationID, id)
}

func (u *UseCase) applyPatch(ctx context.Context, s entity.Subject, cur, next entity.Connection, p entity.ConnectionPatch, now time.Time) error {
	reconfigured := p.AdapterURL != nil || p.AdapterKey != nil
	if p.Name != nil {
		next.Name = *p.Name
	}
	if p.SyncPaused != nil {
		next.SyncPaused = *p.SyncPaused
	}
	if p.BaseURL != nil {
		inst, err := u.d.Repo.EnsureInstance(ctx, s.OrganizationID, cur.Provider, instanceKey(*p.BaseURL))
		if err != nil {
			return err
		}
		if err := u.d.Repo.SetInstanceBaseURL(ctx, s.OrganizationID, inst.ID, *p.BaseURL); err != nil {
			return err
		}
		reconfigured = reconfigured || inst.ID != cur.InstanceID
		next.InstanceID = inst.ID
	}
	var added []entity.NewScope
	var removed []entity.ID
	if p.Scopes != nil {
		scopes, err := u.d.Repo.Scopes(ctx, s.OrganizationID, cur.ID)
		if err != nil {
			return err
		}
		var history []entity.ConnectionScope
		added, removed, history = diffScopes(scopes, *p.Scopes)
		for i := range history {
			if err := u.d.Repo.SetScopeHistory(ctx, s.OrganizationID, history[i].ID, history[i].HistoryFrom); err != nil {
				return err
			}
		}
		reconfigured = reconfigured || len(added) > 0 || len(removed) > 0 || len(history) > 0
	}
	if reconfigured && (next.Status == entity.ConnectionActive || next.Status == entity.ConnectionAccessLost) {
		next.Status, next.AccessLostAt = entity.ConnectionDraft, nil
	}
	if reconfigured || len(p.Credentials) > 0 {
		next.RowVersion++
	}
	if err := u.d.Repo.SaveConnection(ctx, next); err != nil {
		return err
	}
	for _, id := range removed {
		if err := u.d.Repo.RemoveScope(ctx, s.OrganizationID, id, now); err != nil {
			return err
		}
	}
	for _, sc := range added {
		if _, err := u.d.Repo.AddScope(ctx, s.OrganizationID, next.ID, next.InstanceID, named(sc), now); err != nil {
			return err
		}
	}
	return u.audit(ctx, s, "updateConnection", cur.ID, strconv.Itoa(cur.RowVersion), strconv.Itoa(next.RowVersion))
}

// diffScopes compares the active scopes with the wanted set by kind and source ID.
func diffScopes(have []entity.ConnectionScope, want []entity.NewScope) (added []entity.NewScope, removed []entity.ID, history []entity.ConnectionScope) {
	active := make(map[[2]string]entity.ConnectionScope, len(have))
	for i := range have {
		if sc := &have[i]; sc.Active {
			active[[2]string{string(sc.Kind), sc.NativeScopeID}] = *sc
		}
	}
	kept := make(map[[2]string]bool, len(want))
	for _, w := range want {
		k := [2]string{string(w.Kind), w.NativeScopeID}
		kept[k] = true
		sc, ok := active[k]
		switch {
		case !ok:
			added = append(added, w)
		case !sameTime(sc.HistoryFrom, w.HistoryFrom):
			sc.HistoryFrom = w.HistoryFrom
			history = append(history, sc)
		}
	}
	for k := range active {
		if !kept[k] {
			removed = append(removed, active[k].ID)
		}
	}
	slices.SortFunc(removed, func(a, b entity.ID) int { return strings.Compare(a.String(), b.String()) })
	return added, removed, history
}

// named shows the source ID of a scope until a probe of the source reads its name.
func named(sc entity.NewScope) entity.NewScope {
	if sc.DisplayName == "" {
		sc.DisplayName = sc.NativeScopeID
	}
	return sc
}

func sameTime(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

// Delete closes the connection and deletes its credentials in the adapter. A repeat finds it deleted and
// asks the adapter again, so credentials left after a failed call are removed too.
func (u *UseCase) Delete(ctx context.Context, s entity.Subject, id entity.ID) error {
	var c entity.Connection
	err := u.d.Tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		if c, err = u.d.Repo.LockConnection(ctx, s.OrganizationID, id); err != nil {
			return err
		}
		if c.Status == entity.ConnectionDeleted {
			return nil
		}
		now := u.d.Clock.Now()
		c.Status, c.DeletedAt, c.AccessLostAt = entity.ConnectionDeleted, &now, nil
		if err := u.d.Repo.RemoveScopes(ctx, s.OrganizationID, id, now); err != nil {
			return err
		}
		if err := u.d.Repo.SaveConnection(ctx, c); err != nil {
			return err
		}
		return u.audit(ctx, s, "deleteConnection", id, strconv.Itoa(c.RowVersion), "")
	})
	if err != nil {
		return err
	}
	return u.invoke(ctx, c, entity.OpDeleteCredentials, func(ctx context.Context, t entity.AdapterTarget) error {
		return u.d.Adapters.DeleteCredentials(ctx, t, entity.CredentialsFilter{})
	})
}

func (u *UseCase) connection(ctx context.Context, orgID, id entity.ID) (entity.Connection, error) {
	var c entity.Connection
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		c, err = u.d.Repo.Connection(ctx, orgID, id)
		return err
	})
	return c, err
}

// provisionalInstance is the instance by the normalized address until a probe names its stable key.
func (u *UseCase) provisionalInstance(ctx context.Context, orgID entity.ID, p entity.Provider, baseURL string) (entity.SourceInstance, error) {
	var out entity.SourceInstance
	err := u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		if out, err = u.d.Repo.EnsureInstance(ctx, orgID, p, instanceKey(baseURL)); err != nil {
			return err
		}
		return u.d.Repo.SetInstanceBaseURL(ctx, orgID, out.ID, baseURL)
	})
	return out, err
}

// instanceKey normalizes an address: the scheme and host in lower case, no default port, no trailing slash.
func instanceKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && port != _defaultPorts[u.Scheme] {
		host += ":" + port
	}
	return strings.ToLower(u.Scheme) + "://" + host + strings.TrimRight(u.EscapedPath(), "/")
}

var _defaultPorts = map[string]string{"https": "443", "ssh": "22"}

func (u *UseCase) sealKey(c *entity.Connection, rawURL string, key entity.Secret) error {
	blob, version, err := u.d.Keyring.SealEnvelope([]byte(key.Value()), AdapterKeyAAD(c.OrganizationID, c.ID))
	if err != nil {
		return fmt.Errorf("connect: seal adapter key: %w", err)
	}
	c.AdapterURL, c.AdapterKeyEnc, c.KeyVersion = rawURL, blob, version
	return nil
}

func (u *UseCase) adapterKey(c entity.Connection) (entity.Secret, error) {
	key, err := u.d.Keyring.OpenEnvelope(c.AdapterKeyEnc, c.KeyVersion, AdapterKeyAAD(c.OrganizationID, c.ID))
	if err != nil {
		return "", fmt.Errorf("connect: open adapter key: %w", err)
	}
	return entity.Secret(key), nil
}

// sendCredentials sends the fields of the form as one JSON value of the collect purpose: the adapter knows
// its fields, the core does not keep them.
func (u *UseCase) sendCredentials(ctx context.Context, c entity.Connection, fields map[string]entity.Secret) error {
	plain := make(map[string]string, len(fields))
	for k, v := range fields {
		plain[k] = v.Value()
	}
	raw, err := json.Marshal(plain)
	if err != nil {
		return fmt.Errorf("connect: encode credentials: %w", err)
	}
	return u.invoke(ctx, c, entity.OpPutCredentials, func(ctx context.Context, t entity.AdapterTarget) error {
		return u.d.Adapters.PutCredentials(ctx, t, entity.CredentialsInput{Purpose: "collect", Secret: entity.Secret(raw)})
	})
}

// dropCredentials removes credentials of a connection that was not written; the request may be gone.
func (u *UseCase) dropCredentials(ctx context.Context, c entity.Connection) {
	ctx = context.WithoutCancel(ctx)
	err := u.invoke(ctx, c, entity.OpDeleteCredentials, func(ctx context.Context, t entity.AdapterTarget) error {
		return u.d.Adapters.DeleteCredentials(ctx, t, entity.CredentialsFilter{})
	})
	if err != nil {
		logFailure(ctx, c, entity.OpDeleteCredentials, err)
	}
}

func (u *UseCase) audit(ctx context.Context, s entity.Subject, operation string, id entity.ID, base, next string) error {
	return u.d.Auditor.Record(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID, OccurredAt: u.d.Clock.Now(), ActorKind: entity.ActorUser, ActorID: s.UserID,
		Operation: operation, ObjectKind: "connection", ObjectID: id, Outcome: entity.AuditSucceeded,
		BaseVersion: base, NewVersion: next, CorrelationID: entity.ID(logger.CorrelationID(ctx)),
	})
}

func (u *UseCase) validateCreate(ctx context.Context, in entity.ConnectionInput) error {
	v := &entity.ValidationError{}
	checkName(v, in.Name)
	builtin := in.AdapterKind == entity.AdapterBuiltin
	info, cataloged := catalogEntry(in.Provider)
	switch {
	case !slices.Contains(_providers, in.Provider):
		v.Add("/provider", entity.FieldNotAllowedValue, "")
	case builtin && (!cataloged || !u.d.Builtin(in.Provider)):
		v.Add("/provider", entity.FieldNotAllowedValue, "the installation has no built-in adapter of the provider")
	}
	checkBaseURL(v, in.Provider, in.BaseURL)
	switch in.AdapterKind {
	case entity.AdapterBuiltin:
		if in.AdapterURL != "" {
			v.Add("/adapter_url", entity.FieldNotAllowedValue, "a built-in adapter has the address of the installation")
		}
		if in.AdapterKey != "" {
			v.Add("/adapter_key", entity.FieldNotAllowedValue, "a built-in adapter has the key of the installation")
		}
	case entity.AdapterClient:
		checkAdapter(v, in.AdapterURL, in.AdapterKey, true)
	default:
		v.Add("/adapter_kind", entity.FieldNotAllowedValue, "")
	}
	checkCredentials(v, in.Credentials, info, builtin && cataloged, true)
	if len(in.Scopes) == 0 {
		v.Add("/scopes", entity.FieldRequired, "")
	}
	checkScopes(v, in.Scopes, info, builtin && cataloged)
	if len(v.Fields) > 0 {
		return v
	}
	return u.checkAddresses(ctx, in.BaseURL, in.AdapterURL)
}

func (u *UseCase) validatePatch(ctx context.Context, cur entity.Connection, p entity.ConnectionPatch) error {
	v := &entity.ValidationError{}
	if p.Name != nil {
		checkName(v, *p.Name)
	}
	if p.BaseURL != nil {
		checkBaseURL(v, cur.Provider, *p.BaseURL)
	}
	info, cataloged := catalogEntry(cur.Provider)
	builtin := cur.AdapterKind == entity.AdapterBuiltin
	if builtin && p.AdapterURL != nil {
		v.Add("/adapter_url", entity.FieldNotAllowedValue, "a built-in adapter has the address of the installation")
	}
	if builtin && p.AdapterKey != nil {
		v.Add("/adapter_key", entity.FieldNotAllowedValue, "a built-in adapter has the key of the installation")
	}
	if !builtin && (p.AdapterURL != nil || p.AdapterKey != nil) {
		rawURL, key := cur.AdapterURL, entity.Secret("")
		if p.AdapterURL != nil {
			rawURL = *p.AdapterURL
		}
		if p.AdapterKey != nil {
			key = *p.AdapterKey
		}
		checkAdapter(v, rawURL, key, p.AdapterKey != nil)
	}
	checkCredentials(v, p.Credentials, info, builtin && cataloged, false)
	if p.Scopes != nil {
		if len(*p.Scopes) == 0 {
			v.Add("/scopes", entity.FieldRequired, "a connection collects at least one scope")
		}
		checkScopes(v, *p.Scopes, info, builtin && cataloged)
	}
	if len(v.Fields) > 0 {
		return v
	}
	var baseURL, adapterURL string
	if p.BaseURL != nil {
		baseURL = *p.BaseURL
	}
	if p.AdapterURL != nil {
		adapterURL = *p.AdapterURL
	}
	return u.checkAddresses(ctx, baseURL, adapterURL)
}

func (u *UseCase) checkAddresses(ctx context.Context, baseURL, adapterURL string) error {
	v := &entity.ValidationError{}
	for _, c := range []struct {
		field, url string
		check      func(context.Context, string) error
	}{
		{"/base_url", baseURL, u.d.Addresses.CheckSourceURL},
		{"/adapter_url", adapterURL, u.d.Addresses.CheckAdapterURL},
	} {
		if c.url == "" {
			continue
		}
		err := c.check(ctx, c.url)
		if errors.Is(err, entity.ErrOutsideScope) {
			v.Add(c.field, entity.FieldOutsideScope, "the address is outside the network scope of the installation")
			continue
		}
		if err != nil {
			return err
		}
	}
	if len(v.Fields) > 0 {
		return v
	}
	return nil
}

func checkName(v *entity.ValidationError, name string) {
	switch n := utf8.RuneCountInString(name); {
	case strings.TrimSpace(name) == "":
		v.Add("/name", entity.FieldRequired, "")
	case n > _maxName:
		v.Add("/name", entity.FieldTooLong, "")
	}
}

func checkBaseURL(v *entity.ValidationError, p entity.Provider, raw string) {
	u, err := url.Parse(raw)
	switch {
	case raw == "":
		v.Add("/base_url", entity.FieldRequired, "")
	case len(raw) > _maxURL:
		v.Add("/base_url", entity.FieldTooLong, "")
	case err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "":
		v.Add("/base_url", entity.FieldInvalidFormat, "an https address without credentials, query and fragment")
	case u.Scheme == "ssh" && p != entity.ProviderGit:
		v.Add("/base_url", entity.FieldInvalidFormat, "ssh is accepted only by the git adapter")
	case u.Scheme != "https" && u.Scheme != "ssh":
		v.Add("/base_url", entity.FieldInvalidFormat, "an https address")
	}
}

func checkAdapter(v *entity.ValidationError, rawURL string, key entity.Secret, keyGiven bool) {
	u, err := url.Parse(rawURL)
	switch {
	case rawURL == "":
		v.Add("/adapter_url", entity.FieldRequired, "an adapter of a client needs its address")
	case len(rawURL) > _maxURL:
		v.Add("/adapter_url", entity.FieldTooLong, "")
	case err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "":
		v.Add("/adapter_url", entity.FieldInvalidFormat, "an https address without credentials and query")
	}
	switch {
	case keyGiven && key == "":
		v.Add("/adapter_key", entity.FieldRequired, "an adapter of a client needs its key")
	case len(key) > _maxAdapterKey:
		v.Add("/adapter_key", entity.FieldTooLong, "")
	}
}

// checkCredentials checks the fields of a built-in adapter by its catalog entry; on a patch absent
// fields are left unchanged in the adapter, so only names and sizes are checked.
func checkCredentials(v *entity.ValidationError, fields map[string]entity.Secret, info entity.AdapterInfo, cataloged, creating bool) {
	if len(fields) > _maxCredentialFields {
		v.Add("/credentials", entity.FieldOutOfRange, "at most 16 fields")
	}
	names := make([]string, 0, len(fields))
	for k := range fields {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		switch {
		case cataloged && !slices.ContainsFunc(info.CredentialFields, func(f entity.CredentialField) bool { return f.Name == k }):
			v.Add("/credentials/"+k, entity.FieldUnknownField, "")
		case len(fields[k]) > _maxCredentialValue:
			v.Add("/credentials/"+k, entity.FieldTooLong, "")
		}
	}
	if !cataloged || (!creating && len(fields) == 0) {
		return
	}
	for _, f := range info.CredentialFields {
		if f.Required && fields[f.Name] == "" {
			v.Add("/credentials/"+f.Name, entity.FieldRequired, "")
		}
	}
}

func checkScopes(v *entity.ValidationError, scopes []entity.NewScope, info entity.AdapterInfo, cataloged bool) {
	if len(scopes) > _maxScopes {
		v.Add("/scopes", entity.FieldOutOfRange, "at most 500 scopes")
		return
	}
	seen := make(map[[2]string]bool, len(scopes))
	for i, sc := range scopes {
		field := "/scopes/" + strconv.Itoa(i)
		switch {
		case !slices.Contains(_scopeKinds, sc.Kind), cataloged && !slices.Contains(info.ScopeKinds, sc.Kind):
			v.Add(field+"/kind", entity.FieldNotAllowedValue, "")
		case sc.NativeScopeID == "" || len(sc.NativeScopeID) > _maxScopeID:
			v.Add(field+"/native_scope_id", entity.FieldOutOfRange, "1 to 255 bytes")
		case seen[[2]string{string(sc.Kind), sc.NativeScopeID}]:
			v.Add(field, entity.FieldDuplicate, "")
		}
		seen[[2]string{string(sc.Kind), sc.NativeScopeID}] = true
	}
}
