package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const (
	_operationKey = "operation_id"
	_unmatched    = "unmatched"
)

func Correlate(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(logger.WithCorrelation(c.Request.Context(), log))
		c.Next()
	}
}

func Observe(metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)

		op := c.GetString(_operationKey)
		if op == "" {
			op = _unmatched
		}
		status := c.Writer.Status()
		metrics.observe(op, status, elapsed)

		log := zerolog.Ctx(c.Request.Context())
		event := log.Info()
		if status >= http.StatusInternalServerError {
			event = log.Error()
		}
		if err := c.Errors.Last(); err != nil {
			event = event.Err(err.Err)
		}
		event.
			Str("operation_id", op).
			Str("method", c.Request.Method).
			Str("route", c.FullPath()).
			Int("status", status).
			Dur("duration_ms", elapsed).
			Msg("request finished")
	}
}

func Recovery() gin.HandlerFunc {
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
				Msg("request panicked")
			if c.Writer.Written() {
				c.Abort()
				return
			}
			response.Write(c, response.InternalError)
		}()
		c.Next()
	}
}

func LimitBody(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > limit {
			response.Write(c, response.PayloadTooLarge)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
