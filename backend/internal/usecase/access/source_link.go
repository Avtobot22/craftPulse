package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _sourceLinksPath = "/source-links"

type LinkSources interface {
	SourceConnections(context.Context, entity.ID) ([]entity.SourceConnection, error)
	AuthorizeURL(context.Context, entity.ID, entity.ID, entity.SourceAuthorization) (string, error)
	PutCredentials(context.Context, entity.ID, entity.ID, entity.CredentialsInput) error
	DeleteCredentials(context.Context, entity.ID, entity.ID, entity.CredentialsFilter) error
}

type LinkAuditor interface {
	Record(context.Context, entity.AuditEvent) error
}

func (u *UseCase) ListSourceLinks(ctx context.Context, s entity.Subject) ([]entity.SourceLink, error) {
	connections, err := u.visibleSources(ctx, s)
	if err != nil {
		return nil, err
	}
	links, err := u.savedLinks(ctx, s)
	if err != nil {
		return nil, err
	}
	out := make([]entity.SourceLink, 0, len(connections))
	for _, c := range connections {
		if c.AccessMode != entity.AccessUserToken || c.Status != entity.ConnectionActive {
			continue
		}
		l := entity.SourceLink{State: "not_linked"}
		for _, existing := range links {
			if existing.ConnectionID == c.ID {
				l = existing
				break
			}
		}
		l.ConnectionID, l.Provider, l.DisplayName = c.ID, c.Provider, c.Name
		out = append(out, l)
	}
	return out, nil
}

func (u *UseCase) StartSourceLink(ctx context.Context, s entity.Subject, conn entity.ID, returnTo, redirectURI string) (string, error) {
	if err := u.sourceForLink(ctx, s, conn, true); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	state := base64.RawURLEncoding.EncodeToString(raw)
	at := u.clock.Now()
	a := entity.SourceLinkAttempt{
		StateHash: sourceStateHash(state), UserID: s.UserID, SessionID: s.SessionID,
		ConnectionID: conn, ReturnPath: _sourceLinksPath, RedirectURI: redirectURI, CreatedAt: at, ExpiresAt: at.Add(10 * time.Minute),
	}
	if returnTo != "" {
		a.ReturnPath = entity.ReturnPath(returnTo)
	}
	authorization, err := u.links.AuthorizeURL(ctx, s.OrganizationID, conn, entity.SourceAuthorization{UserID: s.UserID, State: state, RedirectURI: redirectURI})
	if err != nil {
		return "", err
	}
	err = u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		a.Version, err = u.linkRepo.ReserveSourceLink(ctx, s.OrganizationID, s.UserID, conn)
		if err != nil {
			return err
		}
		return u.linkRepo.CreateSourceLinkAttempt(ctx, s.OrganizationID, a)
	})
	return authorization, err
}

func (u *UseCase) CompleteSourceLink(ctx context.Context, s entity.Subject, cb entity.SourceLinkCallback) string {
	var a entity.SourceLinkAttempt
	var found bool
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		a, found, err = u.linkRepo.TakeSourceLinkAttempt(ctx, s.OrganizationID, s.UserID, s.SessionID, sourceStateHash(cb.State))
		return err
	})
	if err != nil || !found {
		return linkResult(_sourceLinksPath, "link_failed")
	}
	failed := linkResult(a.ReturnPath, "link_failed")
	if cb.Error != "" || cb.Code == "" || !u.clock.Now().Before(a.ExpiresAt) {
		return failed
	}
	if err := u.sourceForLink(ctx, s, a.ConnectionID, true); err != nil {
		return failed
	}
	prepared := false
	err = u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		prepared, err = u.linkRepo.PrepareSourceLinkExchange(ctx, s.OrganizationID, s.UserID, a.ConnectionID, a.Version)
		return err
	})
	if err != nil || !prepared {
		return failed
	}
	// A replaced adapter token cannot open the old grant if confirmation or its audit fails.
	u.checks.invalidate(s.OrganizationID, s.UserID, a.ConnectionID)
	err = u.links.PutCredentials(ctx, s.OrganizationID, a.ConnectionID, entity.CredentialsInput{
		Purpose: "user_token", UserID: &s.UserID, OAuthCode: cb.Code, RedirectURI: a.RedirectURI,
	})
	if err != nil {
		return failed
	}
	confirmed := false
	err = u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		confirmed, err = u.linkRepo.ConfirmSourceLink(ctx, s.OrganizationID, s.UserID, a.ConnectionID, a.Version, u.clock.Now())
		if err != nil || !confirmed {
			return err
		}
		return u.linkAudit(ctx, s, a.ConnectionID, "source_link.link", "linked")
	})
	u.checks.invalidate(s.OrganizationID, s.UserID, a.ConnectionID)
	if err != nil || !confirmed {
		return failed
	}
	return linkResult(a.ReturnPath, "linked")
}

func (u *UseCase) DeleteSourceLink(ctx context.Context, s entity.Subject, conn entity.ID) error {
	if err := u.sourceForLink(ctx, s, conn, false); err != nil {
		return err
	}
	// Close reads before the remote deletion; an unavailable adapter must not leave cached access open.
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		changed, err := u.linkRepo.RevokeSourceLink(ctx, s.OrganizationID, s.UserID, conn)
		if err != nil || !changed {
			return err
		}
		return u.linkAudit(ctx, s, conn, "source_link.revoke", "revoked")
	})
	if err != nil {
		return err
	}
	u.checks.invalidate(s.OrganizationID, s.UserID, conn)
	return u.links.DeleteCredentials(ctx, s.OrganizationID, conn, entity.CredentialsFilter{Purpose: "user_token", UserID: &s.UserID})
}

func (u *UseCase) visibleSources(ctx context.Context, s entity.Subject) ([]entity.SourceConnection, error) {
	if s.SessionID.IsZero() || s.UserID.IsZero() || !s.UploadTokenID.IsZero() {
		return nil, entity.ErrAuthRequired
	}
	p, err := u.profile(ctx, s)
	if err != nil {
		return nil, err
	}
	connections, err := u.links.SourceConnections(ctx, s.OrganizationID)
	if err != nil {
		return nil, err
	}
	var links []entity.Link
	err = u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		links, err = u.structure.ProjectAreas(ctx, s.OrganizationID, u.clock.Now())
		return err
	})
	if err != nil {
		return nil, err
	}
	allowed := scope(p).Projects()
	return slices.DeleteFunc(connections, func(c entity.SourceConnection) bool {
		return !p.Whole && !slices.ContainsFunc(links, func(l entity.Link) bool {
			return slices.Contains(allowed, l.From) && slices.Contains(c.Areas, l.To)
		})
	}), nil
}

func (u *UseCase) sourceForLink(ctx context.Context, s entity.Subject, conn entity.ID, active bool) error {
	connections, err := u.visibleSources(ctx, s)
	if err != nil {
		return err
	}
	for _, c := range connections {
		if c.ID != conn {
			continue
		}
		if c.AccessMode != entity.AccessUserToken {
			if !active {
				return entity.ErrNotFound
			}
			return entity.ErrStateConflict
		}
		if active && c.Status != entity.ConnectionActive {
			return entity.ErrStateConflict
		}
		return nil
	}
	return entity.ErrNotFound
}

func (u *UseCase) savedLinks(ctx context.Context, s entity.Subject) ([]entity.SourceLink, error) {
	var links []entity.SourceLink
	err := u.tx.InOrgTx(ctx, s.OrganizationID, func(ctx context.Context) error {
		var err error
		links, err = u.linkRepo.SourceLinks(ctx, s.OrganizationID, s.UserID)
		return err
	})
	return links, err
}

func (u *UseCase) linkAudit(ctx context.Context, s entity.Subject, conn entity.ID, operation, state string) error {
	return u.linkAuditor.Record(ctx, entity.AuditEvent{
		OrganizationID: s.OrganizationID, ActorKind: entity.ActorUser,
		ActorID: s.UserID, Operation: operation, ObjectKind: "connection", ObjectID: conn, Outcome: entity.AuditSucceeded,
		CorrelationID: entity.ID(logger.CorrelationID(ctx)), Details: map[string]any{"link_state": state},
	})
}

func sourceStateHash(state string) []byte { sum := sha256.Sum256([]byte(state)); return sum[:] }

func linkResult(returnTo, result string) string {
	p, _ := url.Parse(entity.ReturnPath(returnTo))
	q := p.Query()
	q.Set("source_link_result", result)
	p.RawQuery = q.Encode()
	return p.String()
}

func (u *UseCase) expireLink(ctx context.Context, key checkKey, t target) {
	_ = u.tx.InOrgTx(ctx, key.org, func(ctx context.Context) error {
		return u.linkRepo.ExpireSourceLink(ctx, key.org, key.user, key.connection, t.linkVersion)
	}) // The read remains closed even if recording expiry fails.
}
