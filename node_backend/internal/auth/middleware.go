package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func ControlPlaneAuth(nodeToken, sharedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		controlToken := c.GetHeader("X-Control-Plane-Token")

		if controlToken != sharedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid control plane token"})
			return
		}

		// Backward compatibility: allow control-plane requests through even if the
		// panel still has a stale per-node bearer token stored in its database.
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			if strings.TrimPrefix(authHeader, "Bearer ") == nodeToken {
				c.Next()
				return
			}
		}

		c.Next()
	}
}
