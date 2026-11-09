package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// returns a Gin middleware function for JWT authentication
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization") // Read the Authorization header from the request
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"}) // Reject request if Authorization header is missing
			c.Abort() // Stop request chain
			return
		}

		// Remove "Bearer " prefix to extract raw token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Store user info for handlers to use
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}