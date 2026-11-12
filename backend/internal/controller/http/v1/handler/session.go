package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/cookie"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type session struct {
	logins   usecase.Logins
	access   usecase.Access
	failures *prometheus.CounterVec
}

func newSession(logins usecase.Logins, access usecase.Access, reg prometheus.Registerer) session {
	failures := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "craftpulse_auth_login_failures_total",
		Help: "Failed logins by provider issuer and failure class.",
	}, []string{"provider", "reason"})
	reg.MustRegister(failures)
	return session{logins: logins, access: access, failures: failures}
}

func (h *session) StartLogin(ctx context.Context, req openapi.StartLoginRequestObject) (openapi.StartLoginResponseObject, error) {
	start, err := h.logins.StartLogin(ctx, req.OrgSlug, value(req.Params.ReturnTo))
	if err != nil {
		return h.loginFailed(ctx, req.OrgSlug, err), nil
	}
	return redirect{location: start.RedirectURL, cookies: func(w http.ResponseWriter) {
		cookie.Set(w, cookie.Login, start.Cookie, entity.LoginAttemptLifetime)
	}}, nil
}

func (h *session) CompleteLogin(ctx context.Context, req openapi.CompleteLoginRequestObject) (openapi.CompleteLoginResponseObject, error) {
	res, err := h.logins.CompleteLogin(ctx, entity.LoginCallback{
		Slug:          req.OrgSlug,
		LoginCookie:   requestCookie(ctx, cookie.Login),
		SessionCookie: requestCookie(ctx, cookie.Session),
		Code:          value(req.Params.Code),
		State:         req.Params.State,
		Issuer:        value(req.Params.Iss),
		Error:         value(req.Params.Error),
	})
	if err != nil {
		return h.loginFailed(ctx, req.OrgSlug, err), nil
	}
	return redirect{location: res.ReturnPath, cookies: func(w http.ResponseWriter) {
		cookie.Clear(w, cookie.Login)
		cookie.Set(w, cookie.Session, res.SessionToken, entity.SessionLifetime)
	}}, nil
}

func (h *session) Logout(ctx context.Context, _ openapi.LogoutRequestObject) (openapi.LogoutResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if err := h.logins.Logout(ctx, s); err != nil {
		return nil, err
	}
	return loggedOut{}, nil
}

func (h *session) GetCurrentUser(ctx context.Context, _ openapi.GetCurrentUserRequestObject) (openapi.GetCurrentUserResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	cu, err := h.access.CurrentUser(ctx, s)
	if err != nil {
		return nil, err
	}
	return openapi.GetCurrentUser200JSONResponse(response.CurrentUser(cu)), nil
}

// loginFailed sends the browser to the login page of the organization with the failure class (UI-03).
// The use case has logged a login step failure; any other error is logged here.
func (h *session) loginFailed(ctx context.Context, slug string, err error) redirect {
	class, provider := entity.LoginProviderUnavailable, ""
	if le, ok := errors.AsType[*entity.LoginError](err); ok {
		class, provider = le.Class, le.Issuer
	} else if c, ok := ctx.(*gin.Context); ok {
		_ = c.Error(err)
	}
	if provider == "" {
		provider = "unknown"
	}
	h.failures.WithLabelValues(provider, string(class)).Inc()
	q := url.Values{"login_error": {string(class)}}
	return redirect{location: "/o/" + url.PathEscape(slug) + "?" + q.Encode(), cookies: func(w http.ResponseWriter) {
		cookie.Clear(w, cookie.Login)
	}}
}

type redirect struct {
	location string
	cookies  func(w http.ResponseWriter)
}

func (r redirect) VisitStartLoginResponse(w http.ResponseWriter) error { return r.visit(w) }

func (r redirect) VisitCompleteLoginResponse(w http.ResponseWriter) error { return r.visit(w) }

func (r redirect) visit(w http.ResponseWriter) error {
	r.cookies(w)
	w.Header().Set("Location", r.location)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
	return nil
}

type loggedOut struct{}

func (loggedOut) VisitLogoutResponse(w http.ResponseWriter) error {
	cookie.Clear(w, cookie.Session)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// requestCookie reads a cookie the contract does not declare; strict handlers get *gin.Context as ctx.
func requestCookie(ctx context.Context, name string) string {
	c, ok := ctx.(*gin.Context)
	if !ok {
		return ""
	}
	v, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return v
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
