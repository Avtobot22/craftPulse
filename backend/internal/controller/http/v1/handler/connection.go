package handler

import (
	"context"
	"math"
	"net/http"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

const _connectionCursor = "connection"

type connections struct {
	uc         usecase.Connections
	sync       usecase.SyncStatus
	operations usecase.Operations
}

func (h *connections) ListAdapters(context.Context, openapi.ListAdaptersRequestObject) (openapi.ListAdaptersResponseObject, error) {
	adapters := h.uc.ListAdapters()
	out := openapi.AdapterList{Data: make([]openapi.Adapter, 0, len(adapters))}
	for i := range adapters {
		a := &adapters[i]
		item := openapi.Adapter{
			Provider: openapi.Provider(a.Provider), DisplayName: a.DisplayName, Editions: a.Editions, ContractVersions: a.ContractVersions,
			DataKinds: make([]openapi.DataKind, 0, len(a.DataKinds)), ScopeKinds: make([]openapi.AdapterScopeKinds, 0, len(a.ScopeKinds)),
		}
		for _, k := range a.DataKinds {
			item.DataKinds = append(item.DataKinds, openapi.DataKind(k))
		}
		for _, k := range a.ScopeKinds {
			item.ScopeKinds = append(item.ScopeKinds, openapi.AdapterScopeKinds(k))
		}
		for _, f := range a.CredentialFields {
			item.CredentialFields = append(item.CredentialFields, struct {
				Label    string `json:"label"`
				Name     string `json:"name"`
				Required bool   `json:"required"`
				Secret   bool   `json:"secret"`
			}{Label: f.Label, Name: f.Name, Required: f.Required, Secret: f.Secret})
		}
		out.Data = append(out.Data, item)
	}
	return openapi.ListAdapters200JSONResponse(out), nil
}

func (h *connections) ListConnections(ctx context.Context, req openapi.ListConnectionsRequestObject) (openapi.ListConnectionsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	f := entity.ConnectionFilter{Limit: limit}
	if req.Params.Status != nil {
		f.Status = entity.ConnectionStatus(*req.Params.Status)
	}
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != _connectionCursor || c.Name != string(f.Status) || c.At.IsZero() || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		f.After = &entity.AuditKey{OccurredAt: c.At, ID: entity.ID(c.ID)}
	}
	page, err := h.uc.List(ctx, s, f)
	if err != nil {
		return nil, err
	}
	out := openapi.ConnectionPage{Data: make([]openapi.Connection, 0, len(page.Items))}
	for i := range page.Items {
		out.Data = append(out.Data, connectionBody(page.Items[i]))
	}
	if err := h.withLastSuccess(ctx, s.OrganizationID, out.Data); err != nil {
		return nil, err
	}
	if page.Next != nil {
		c := encodeCursor(cursorData{Kind: _connectionCursor, Name: string(f.Status), At: page.Next.OccurredAt, ID: uuid.UUID(page.Next.ID)})
		out.NextCursor = &c
	}
	return openapi.ListConnections200JSONResponse(out), nil
}

func (h *connections) CreateConnection(ctx context.Context, req openapi.CreateConnectionRequestObject) (openapi.CreateConnectionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	b := req.Body
	if b == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/name", Code: entity.FieldRequired}}}
	}
	in := entity.ConnectionInput{
		OrganizationID: s.OrganizationID, CreatedBy: s.UserID, Name: b.Name, Provider: entity.Provider(b.Provider),
		BaseURL: b.BaseUrl, AdapterKind: entity.AdapterKind(b.AdapterKind), Scopes: scopeInputs(b.Scopes),
	}
	if b.AdapterUrl != nil {
		in.AdapterURL = *b.AdapterUrl
	}
	if b.AdapterKey != nil {
		in.AdapterKey = entity.Secret(*b.AdapterKey)
	}
	if b.Credentials != nil {
		in.Credentials = credentials(*b.Credentials)
	}
	st, err := h.uc.Create(ctx, s, in)
	if err != nil {
		return nil, err
	}
	tag, location := etag(int64(st.Connection.RowVersion)), "/api/v1/connections/"+st.Connection.ID.String()
	return connectionCreated{openapi.CreateConnection201JSONResponse{
		Body: connectionBody(st), Headers: openapi.CreateConnection201ResponseHeaders{ETag: &tag, Location: &location},
	}}, nil
}

func (h *connections) GetConnection(ctx context.Context, req openapi.GetConnectionRequestObject) (openapi.GetConnectionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	st, err := h.uc.State(ctx, s.OrganizationID, entity.ID(req.ConnectionId))
	if err != nil {
		return nil, err
	}
	tag := etag(int64(st.Connection.RowVersion))
	body := []openapi.Connection{connectionBody(st)}
	if err := h.withLastSuccess(ctx, s.OrganizationID, body); err != nil {
		return nil, err
	}
	return openapi.GetConnection200JSONResponse{Body: body[0], Headers: openapi.GetConnection200ResponseHeaders{ETag: &tag}}, nil
}

func (h *connections) UpdateConnection(ctx context.Context, req openapi.UpdateConnectionRequestObject) (openapi.UpdateConnectionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	base, ok := rowVersion(req.Params.IfMatch)
	if !ok {
		return nil, entity.ErrVersionConflict
	}
	var p entity.ConnectionPatch
	if b := req.Body; b != nil {
		p.Name, p.BaseURL, p.AdapterURL, p.SyncPaused = b.Name, b.BaseUrl, b.AdapterUrl, b.SyncPaused
		if b.AdapterKey != nil {
			key := entity.Secret(*b.AdapterKey)
			p.AdapterKey = &key
		}
		if b.Credentials != nil {
			p.Credentials = credentials(*b.Credentials)
		}
		if b.Scopes != nil {
			scopes := scopeInputs(*b.Scopes)
			p.Scopes = &scopes
		}
	}
	st, err := h.uc.Update(ctx, s, entity.ID(req.ConnectionId), base, p)
	if err != nil {
		return nil, err
	}
	tag := etag(int64(st.Connection.RowVersion))
	return openapi.UpdateConnection200JSONResponse{Body: connectionBody(st), Headers: openapi.UpdateConnection200ResponseHeaders{ETag: &tag}}, nil
}

func (h *connections) DeleteConnection(ctx context.Context, req openapi.DeleteConnectionRequestObject) (openapi.DeleteConnectionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if err := h.uc.Delete(ctx, s, entity.ID(req.ConnectionId)); err != nil {
		return nil, err
	}
	return openapi.DeleteConnection204Response{}, nil
}

func (h *connections) StartConnectionProbe(ctx context.Context, req openapi.StartConnectionProbeRequestObject) (openapi.StartConnectionProbeResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	p, err := h.uc.StartProbe(ctx, s, entity.ID(req.ConnectionId))
	if err != nil {
		return nil, err
	}
	location := "/api/v1/connections/" + req.ConnectionId.String() + "/probes/" + p.ID.String()
	return openapi.StartConnectionProbe202JSONResponse{Body: probeBody(p), Headers: openapi.StartConnectionProbe202ResponseHeaders{Location: &location}}, nil
}

func (h *connections) GetConnectionProbe(ctx context.Context, req openapi.GetConnectionProbeRequestObject) (openapi.GetConnectionProbeResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	p, err := h.uc.GetProbe(ctx, s, entity.ID(req.ConnectionId), entity.ID(req.ProbeId))
	if err != nil {
		return nil, err
	}
	return openapi.GetConnectionProbe200JSONResponse(probeBody(p)), nil
}

func (h *connections) ActivateConnection(ctx context.Context, req openapi.ActivateConnectionRequestObject) (openapi.ActivateConnectionResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/probe_id", Code: entity.FieldRequired}}}
	}
	// A version that is not ours cannot match the current one; the repeat of an activation is still found.
	base, ok := rowVersion(req.Params.IfMatch)
	if !ok {
		base = 0
	}
	st, err := h.uc.Activate(ctx, s, entity.ID(req.ConnectionId), base, entity.ID(req.Body.ProbeId))
	if err != nil {
		return nil, err
	}
	tag := etag(int64(st.Connection.RowVersion))
	return openapi.ActivateConnection200JSONResponse{Body: connectionBody(st), Headers: openapi.ActivateConnection200ResponseHeaders{ETag: &tag}}, nil
}

// connectionCreated keeps the answer out of caches: it follows a request with credentials.
type connectionCreated struct {
	openapi.CreateConnection201JSONResponse
}

func (r connectionCreated) VisitCreateConnectionResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "no-store")
	return r.CreateConnection201JSONResponse.VisitCreateConnectionResponse(w)
}

func rowVersion(ifMatch string) (int, bool) {
	v, ok := parseETag(ifMatch)
	if !ok || v < 1 || v > math.MaxInt32 {
		return 0, false
	}
	return int(v), true
}

func credentials(in openapi.Credentials) map[string]entity.Secret {
	out := make(map[string]entity.Secret, len(in))
	for k, v := range in {
		if v != "" {
			out[k] = entity.Secret(v)
		}
	}
	return out
}

func scopeInputs(in []openapi.ConnectionScopeInput) []entity.NewScope {
	out := make([]entity.NewScope, 0, len(in))
	for _, sc := range in {
		ns := entity.NewScope{Kind: entity.ScopeKind(sc.Kind), NativeScopeID: sc.NativeScopeId, HistoryFrom: sc.HistoryFrom}
		if sc.RepositoryScopeId != nil {
			id := entity.ID(*sc.RepositoryScopeId)
			ns.RepositoryScopeID = &id
		}
		out = append(out, ns)
	}
	return out
}

// connectionBody shows the active scopes; credentials appear only as the date they were set.
func connectionBody(st entity.ConnectionState) openapi.Connection {
	c := st.Connection
	out := openapi.Connection{
		Id: uuid.UUID(c.ID), Name: c.Name, Provider: openapi.Provider(c.Provider), Status: openapi.ConnectionStatus(c.Status),
		AdapterKind: openapi.ConnectionAdapterKind(c.AdapterKind), AccessMode: openapi.ConnectionAccessMode(c.AccessMode),
		CredentialsSetAt: c.CredentialsSetAt, SyncPaused: c.SyncPaused, PausedUntil: c.PausedUntil,
		Scopes: make([]openapi.ConnectionScope, 0, len(st.Scopes)), Capabilities: make([]openapi.Capability, 0, len(st.Capabilities)),
	}
	out.Instance.Id = uuid.UUID(st.Instance.ID)
	out.Instance.BaseUrl, out.Instance.Edition, out.Instance.SourceVersion = optional(st.Instance.BaseURL), optional(st.Instance.Edition), optional(st.Instance.SourceVersion)
	for i := range st.Scopes {
		sc := &st.Scopes[i]
		if !sc.Active {
			continue
		}
		item := openapi.ConnectionScope{
			Id: uuid.UUID(sc.ID), Kind: openapi.ConnectionScopeKind(sc.Kind), NativeScopeId: sc.NativeScopeID,
			DisplayName: sc.DisplayName, HistoryFrom: sc.HistoryFrom, AreaId: uuidPtr(sc.AreaID), RepositoryScopeId: uuidPtr(sc.RepositoryScopeID),
		}
		out.Scopes = append(out.Scopes, item)
	}
	out.Capabilities = capabilities(st.Capabilities)
	return out
}

func probeBody(p entity.Probe) openapi.ConnectionProbe {
	return openapi.ConnectionProbe{
		Id: uuid.UUID(p.ID), Status: openapi.ConnectionProbeStatus(p.Status), StartedAt: p.StartedAt, FinishedAt: p.FinishedAt,
		Edition: optional(p.Edition), SourceVersion: optional(p.SourceVersion), ContractVersion: optional(p.ContractVersion),
		Capabilities: capabilities(p.Capabilities), ReasonCode: optional(p.ReasonCode), RetryAfterSeconds: retryAfter(p),
		ConfigurationVersion: p.ConfigurationVersion,
	}
}

func retryAfter(p entity.Probe) *int {
	if !p.Running() {
		return nil
	}
	n := 3
	return &n
}

func capabilities(in []entity.Capability) []openapi.Capability {
	out := make([]openapi.Capability, 0, len(in))
	for _, c := range in {
		out = append(out, openapi.Capability{
			Capability: openapi.CapabilityCapability(c.Name), State: openapi.CapabilityState(c.State),
			ReasonCode: optional(c.ReasonCode), CheckedAt: c.CheckedAt,
		})
	}
	return out
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func uuidPtr(id *entity.ID) *uuid.UUID {
	if id == nil {
		return nil
	}
	u := uuid.UUID(*id)
	return &u
}
