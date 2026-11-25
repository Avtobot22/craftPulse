package middleware

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

var _apiKeyValue = regexp.MustCompile(`cpk_[A-Za-z0-9_-]{43}`)

func APIKeyAuthentication(keys usecase.APIKeys) gin.HandlerFunc {
	return func(c *gin.Context) {
		if keys == nil {
			c.Next()
			return
		}
		query := _urlEscape.ReplaceAllStringFunc(c.Request.URL.RawQuery, func(part string) string { decoded, _ := url.QueryUnescape(part); return decoded })
		if exposed := _apiKeyValue.FindAllString(query, -1); len(exposed) > 0 {
			for _, value := range exposed {
				if err := keys.RejectExposedAPIKey(c.Request.Context(), value); err != nil {
					_ = c.Error(err)
					response.Write(c, response.InternalError)
					return
				}
			}
			response.Write(c, response.AuthRequired)
			return
		}
		headers := c.Request.Header.Values("Authorization")
		if len(headers) != 1 {
			c.Next()
			return
		}
		scheme, key, ok := strings.Cut(headers[0], " ")
		if !ok || !strings.HasPrefix(key, "cpk_") {
			c.Next()
			return
		}
		if !strings.EqualFold(scheme, "Bearer") || c.GetString(_operationKey) != "listTasks" {
			response.Write(c, response.AuthRequired)
			return
		}
		s, err := keys.AuthenticateAPIKey(c.Request.Context(), key)
		switch {
		case err == nil:
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), subjectKey{}, s))
			c.Next()
		case errors.Is(err, entity.ErrAuthRequired):
			response.Write(c, response.AuthRequired)
		default:
			_ = c.Error(err)
			response.Write(c, response.InternalError)
		}
	}
}
