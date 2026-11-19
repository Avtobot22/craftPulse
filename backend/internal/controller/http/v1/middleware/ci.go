package middleware

import (
	"mime"
	"net/netip"
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/response"
)

func CIIngress(networks []netip.Prefix) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(_operationKey) != "uploadPackage" {
			c.Next()
			return
		}
		address, err := netip.ParseAddr(c.ClientIP())
		if err != nil || !slices.ContainsFunc(networks, func(p netip.Prefix) bool { return p.Contains(address.Unmap()) }) {
			response.Write(c, response.Forbidden)
			return
		}
		media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || media != "application/gzip" {
			response.Write(c, response.UnsupportedMedia)
			return
		}
		c.Next()
	}
}
