package adaptersdk

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const (
	_maxBodyBytes    = 2 << 20
	_maxDetail       = 1024
	_maxCorrelation  = 64
	_correlationKey  = "adaptersdk.correlation_id"
	_contractHeader  = "X-Craftpulse-Contract-Version"
	_orgHeader       = "X-Craftpulse-Organization"
	_correlationHead = "X-Correlation-Id"
)

type Options struct {
	Source Source
	Store  *Store
	// ServiceKeys are accepted keys of the core: the current one and, during rotation, the previous one.
	ServiceKeys []string
	Log         zerolog.Logger
	// Probes serve /livez, /readyz and /version; the contract version is added by the library.
	Probes health.Probes
	// SpoolDir holds Fetch pages while they are built; empty is the system temporary directory.
	SpoolDir string
	// Limits bound a Fetch page; zero is envelope.DefaultLimits.
	Limits envelope.Limits
}

// NewHandler serves the contract of adapter.yaml under /v1 and the state checks at the root. The process
// owns the http.Server with TLS and its shutdown.
func NewHandler(o Options) (http.Handler, error) {
	if o.Source == nil || o.Store == nil {
		return nil, errors.New("adaptersdk: source and store are required")
	}
	keys := make([][sha256.Size]byte, 0, len(o.ServiceKeys))
	for _, k := range o.ServiceKeys {
		if k != "" {
			keys = append(keys, sha256.Sum256([]byte(k)))
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("adaptersdk: a service key is required")
	}
	limits := o.Limits
	if limits == (envelope.Limits{}) {
		limits = envelope.DefaultLimits
	}

	r := gin.New()
	// Strict handlers receive *gin.Context as context.Context; this exposes the request values.
	r.ContextWithFallback = true
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	_ = r.SetTrustedProxies(nil)

	probes := o.Probes
	probes.ContractVersion = CurrentContractVersion
	state := gin.WrapH(health.Handler(probes))
	r.GET("/livez", state)
	r.GET("/readyz", state)
	r.GET("/version", state)

	chain := make([]gin.HandlerFunc, 0, 4)
	chain = append(chain, correlate(o.Log), observe(), recovery())
	r.NoRoute(append(chain, func(c *gin.Context) {
		writeError(c, NewError(AdapterErrorCodeNotFound, "no such operation"))
	})...)

	api := r.Group("/v1", chain...)
	api.Use(authenticate(keys), contractVersion(), limitBody(_maxBodyBytes))
	srv := &server{src: o.Source, store: o.Store, spoolDir: o.SpoolDir, limits: limits}
	strict := NewStrictHandlerWithOptions(srv, nil, StrictGinServerOptions{
		RequestErrorHandlerFunc:  requestError,
		HandlerErrorFunc:         handlerError,
		ResponseErrorHandlerFunc: responseError,
	})
	RegisterHandlersWithOptions(api, strict, GinServerOptions{ErrorHandler: parameterError})
	return r, nil
}

func correlate(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(_correlationHead)
		if id == "" || len(id) > _maxCorrelation || !utf8.ValidString(id) {
			id = uuid.Must(uuid.NewV7()).String()
		}
		c.Set(_correlationKey, id)
		l := log.With().Str("correlation_id", id).Logger()
		c.Request = c.Request.WithContext(l.WithContext(c.Request.Context()))
		c.Next()
	}
}

func observe() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		log := zerolog.Ctx(c.Request.Context())
		event := log.Info()
		if status >= http.StatusInternalServerError {
			event = log.Warn()
		}
		if last := c.Errors.Last(); last != nil {
			if e, ok := errors.AsType[*Error](last.Err); ok {
				event = event.Str("error_code", string(e.Code)).Str("detail", e.Detail).AnErr("cause", e.cause)
			} else {
				event = event.Err(last.Err)
			}
		}
		if org, err := uuid.Parse(c.GetHeader(_orgHeader)); err == nil {
			event = event.Str("organization_id", org.String())
		}
		if conn, err := uuid.Parse(c.Param("connection_id")); err == nil {
			event = event.Str("connection_id", conn.String())
		}
		event.
			Str("method", c.Request.Method).
			Str("route", c.FullPath()).
			Int("status", status).
			Dur("duration_ms", time.Since(start)).
			Msg("call finished")
	}
}

func recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v)
			}
			zerolog.Ctx(c.Request.Context()).Error().
				Str("panic", fmt.Sprint(v)).
				Bytes("stack", debug.Stack()).
				Msg("call panicked")
			if c.Writer.Written() {
				c.Abort()
				return
			}
			writeError(c, NewError(AdapterErrorCodeUnavailable, "adapter failed"))
		}()
		c.Next()
	}
}

// authenticate compares the bearer key with every accepted key in constant time; hashing first makes
// the comparison independent of the key length.
func authenticate(keys [][sha256.Size]byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := sha256.Sum256([]byte(bearer(c.GetHeader("Authorization"))))
		match := 0
		for i := range keys {
			match |= subtle.ConstantTimeCompare(got[:], keys[i][:])
		}
		if match != 1 {
			writeError(c, NewError(AdapterErrorCodeUnauthorized, "service key rejected"))
			return
		}
		c.Next()
	}
}

func bearer(header string) string {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// contractVersion refuses a version the library does not serve; a missing header is left to the
// parameter check of the operation.
func contractVersion() gin.HandlerFunc {
	return func(c *gin.Context) {
		if v := c.GetHeader(_contractHeader); v != "" && v != CurrentContractVersion {
			if len(v) > 16 {
				v = v[:16]
			}
			writeError(c, NewError(AdapterErrorCodeUnsupportedContractVersion, "contract version "+strconv.Quote(v)+" is not supported"))
			return
		}
		c.Next()
	}
}

func limitBody(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func writeError(c *gin.Context, e *Error) {
	_ = c.Error(e)
	if c.Writer.Written() {
		c.Abort()
		return
	}
	if e.RetryAt != nil {
		c.Header("Retry-After", strconv.Itoa(max(0, int(math.Ceil(time.Until(*e.RetryAt).Seconds())))))
	}
	c.Header("Cache-Control", "no-store")
	detail := e.Detail
	if len(detail) > _maxDetail {
		detail = strings.ToValidUTF8(detail[:_maxDetail], "")
	}
	c.AbortWithStatusJSON(e.status(), AdapterError{
		Code: e.Code, Detail: detail, RetryAt: e.RetryAt, CorrelationId: c.GetString(_correlationKey),
	})
}

func parameterError(c *gin.Context, err error, _ int) {
	writeError(c, &Error{Code: AdapterErrorCodeInvalidRequest, Detail: err.Error()})
}

// requestError hides the decoder message: a body may hold credentials.
func requestError(c *gin.Context, err error) {
	writeError(c, &Error{Code: AdapterErrorCodeInvalidRequest, Detail: "request body is invalid", cause: err})
}

func handlerError(c *gin.Context, err error) {
	if e, ok := errors.AsType[*Error](err); ok {
		writeError(c, e)
		return
	}
	writeError(c, &Error{Code: AdapterErrorCodeUnavailable, Detail: "adapter could not complete the call", cause: err})
}

func responseError(c *gin.Context, err error) {
	writeError(c, &Error{Code: AdapterErrorCodeUnavailable, Detail: "adapter could not write the response", cause: err})
}
