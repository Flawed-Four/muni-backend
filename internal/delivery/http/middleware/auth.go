package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/moni-be/internal/delivery/http/response"
	"github.com/Flawed-Four/moni-be/internal/domain"
)

const userIDKey = "user_id"

// Auth validates the Bearer token and stores the user ID in the Gin context.
func Auth(tokens domain.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, ok := strings.Cut(c.GetHeader("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			response.Abort(c, http.StatusUnauthorized, "missing or malformed authorization header")
			return
		}

		userID, err := tokens.Verify(strings.TrimSpace(token))
		if err != nil {
			response.Abort(c, http.StatusUnauthorized, domain.ErrInvalidToken.Error())
			return
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
}

// UserID returns the authenticated user's ID set by Auth.
func UserID(c *gin.Context) string {
	return c.GetString(userIDKey)
}
