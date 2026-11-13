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

var _urlEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)

var _uploadValue = regexp.MustCompile(`cpt_[A-Za-z0-9_-]{43}`)

func UploadAuthentication(tokens usecase.UploadTokens) gin.HandlerFunc {
	return func(c *gin.Context) {
		if tokens == nil {
			c.Next()
			return
		}
		// Decode valid escapes independently so a malformed sibling cannot hide a leaked token.
		exposed := _urlEscape.ReplaceAllStringFunc(c.Request.URL.RawQuery, func(part string) string { decoded, _ := url.QueryUnescape(part); return decoded })
		values := _uploadValue.FindAllString(exposed, -1)
		if len(values) > 0 {
			for _, value := range values {
				if err := tokens.RejectExposedUpload(c.Request.Context(), value); err != nil {
					_ = c.Error(err)
					response.Write(c, response.InternalError)
					return
				}
			}
			response.Write(c, response.AuthRequired)
			return
		}
		auth := c.Request.Header.Values("Authorization")
		if len(auth) == 0 {
			c.Next()
			return
		}
		if len(auth) != 1 {
			response.Write(c, response.AuthRequired)
			return
		}
		scheme, value, ok := strings.Cut(auth[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			response.Write(c, response.AuthRequired)
			return
		}
		s, err := tokens.AuthenticateUpload(c.Request.Context(), value)
		if errors.Is(err, entity.ErrAuthRequired) {
			response.Write(c, response.AuthRequired)
			return
		}
		if err != nil {
			_ = c.Error(err)
			response.Write(c, response.InternalError)
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), subjectKey{}, s))
		c.Next()
	}
}
