package adaptersdk

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/netguard"
)

// CurrentContractVersion is the adapter.yaml version this library serves.
const CurrentContractVersion = "1"

// Error is a contract error (TZ 10.3): the core branches on Code. Detail is for diagnostics and never
// carries credentials or a source body.
type Error struct {
	Code    AdapterErrorCode
	Detail  string
	RetryAt *time.Time
	cause   error
}

func NewError(code AdapterErrorCode, detail string) *Error {
	return &Error{Code: code, Detail: detail}
}

// Preparing defers the call until retryAt without spending an attempt, for work longer than a call.
func Preparing(retryAt time.Time, detail string) *Error {
	return &Error{Code: AdapterErrorCodePreparing, Detail: detail, RetryAt: &retryAt}
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Detail }

func (e *Error) Unwrap() error { return e.cause }

func (e *Error) status() int {
	switch e.Code {
	case AdapterErrorCodeInvalidRequest:
		return http.StatusBadRequest
	case AdapterErrorCodeUnauthorized:
		return http.StatusUnauthorized
	case AdapterErrorCodeAuth, AdapterErrorCodePermissionMissing:
		return http.StatusForbidden
	case AdapterErrorCodeNotFound:
		return http.StatusNotFound
	case AdapterErrorCodeNotSupported, AdapterErrorCodeHeadConflict:
		return http.StatusConflict
	case AdapterErrorCodeUnsupportedContractVersion, AdapterErrorCodeUnsupportedEdition, AdapterErrorCodeInvalidSignature:
		return http.StatusUnprocessableEntity
	case AdapterErrorCodeRateLimited:
		return http.StatusTooManyRequests
	case AdapterErrorCodeInvalidResponse:
		return http.StatusBadGateway
	case AdapterErrorCodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusServiceUnavailable
	}
}

var errResponseTooLarge = errors.New("adaptersdk: source response is too large")

// classifyStatus turns a source response that is not a success into a contract error.
func classifyStatus(resp *http.Response, now time.Time) *Error {
	code := resp.StatusCode
	switch {
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return NewError(AdapterErrorCodeAuth, "source returned "+strconv.Itoa(code))
	case code == http.StatusNotFound:
		return NewError(AdapterErrorCodeNotFound, "source returned 404")
	case code == http.StatusTooManyRequests:
		e := NewError(AdapterErrorCodeRateLimited, "source returned 429")
		e.RetryAt = retryAfter(resp.Header.Get("Retry-After"), now)
		return e
	case code >= http.StatusInternalServerError:
		return NewError(AdapterErrorCodeUnavailable, "source returned "+strconv.Itoa(code))
	default:
		return NewError(AdapterErrorCodeInvalidResponse, "source returned unexpected status "+strconv.Itoa(code))
	}
}

// retryAfter reads Retry-After as delay seconds or an HTTP date; nil if it is absent or malformed.
func retryAfter(v string, now time.Time) *time.Time {
	if v == "" {
		return nil
	}
	if s, err := strconv.ParseInt(v, 10, 64); err == nil && s >= 0 {
		at := now.Add(time.Duration(min(s, math.MaxInt64/int64(time.Second))) * time.Second).UTC().Truncate(time.Second)
		return &at
	}
	if t, err := http.ParseTime(v); err == nil {
		at := t.UTC()
		return &at
	}
	return nil
}

// classifyError turns a failed exchange with the source into a contract error; only the class of the
// cause reaches the core (TZ 13.7).
func classifyError(err error) *Error {
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	wrap := func(code AdapterErrorCode, detail string) *Error {
		return &Error{Code: code, Detail: detail, cause: err}
	}
	if refused, ok := errors.AsType[*netguard.RefusedError](err); ok {
		return wrap(AdapterErrorCodeInvalidRequest, "source address refused: "+string(refused.Reason))
	}
	if errors.Is(err, errResponseTooLarge) {
		return wrap(AdapterErrorCodeInvalidResponse, "source response exceeds the size limit")
	}
	if netErr, ok := errors.AsType[net.Error](err); (ok && netErr.Timeout()) || errors.Is(err, context.DeadlineExceeded) {
		return wrap(AdapterErrorCodeTimeout, "source did not answer in time")
	}
	if isCertificateError(err) {
		return wrap(AdapterErrorCodeUnavailable, "source certificate is not trusted")
	}
	return wrap(AdapterErrorCodeUnavailable, "source is unreachable")
}

func isCertificateError(err error) bool {
	_, verify := errors.AsType[*tls.CertificateVerificationError](err)
	_, authority := errors.AsType[x509.UnknownAuthorityError](err)
	_, host := errors.AsType[x509.HostnameError](err)
	_, invalid := errors.AsType[x509.CertificateInvalidError](err)
	return verify || authority || host || invalid
}

// InvalidResponseError reports a source answer that did not pass the adapter's check.
func InvalidResponseError(format string, args ...any) *Error {
	return NewError(AdapterErrorCodeInvalidResponse, fmt.Sprintf(format, args...))
}
