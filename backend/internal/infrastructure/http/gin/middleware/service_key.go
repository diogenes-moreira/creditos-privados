package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServiceKeyHeader carries a partner service's API key.
const ServiceKeyHeader = "X-API-Key"

// RequireServiceKey authenticates a partner service (not a user) by a shared
// key. The service name is set as "serviceName" for the handler and audit.
func RequireServiceKey(serviceName, key string) gin.HandlerFunc {
	expected := []byte(key)
	return func(c *gin.Context) {
		got := []byte(c.GetHeader(ServiceKeyHeader))
		if len(expected) == 0 || subtle.ConstantTimeCompare(got, expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid service key"})
			return
		}
		c.Set("serviceName", serviceName)
		c.Next()
	}
}
