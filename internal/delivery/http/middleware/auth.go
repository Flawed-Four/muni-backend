package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/muni-backend/internal/delivery/http/response"
	"github.com/Flawed-Four/muni-backend/internal/usecase"
)

const (
	userIDKey = "user_id"
	tokenKey  = "session_token"
)

// Auth validates the Bearer session token and stores the user ID and token in the Gin context.
func Auth(auth *usecase.AuthUsecase) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, ok := strings.Cut(c.GetHeader("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			response.Abort(c, http.StatusUnauthorized, "missing or malformed authorization header")
			return
		}

		token = strings.TrimSpace(token)
		userID, err := auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			response.Fail(c, err)
			return
		}

		c.Set(userIDKey, userID)
		c.Set(tokenKey, token)
		c.Next()
	}
}

// UserID returns the authenticated user's ID set by Auth.
func UserID(c *gin.Context) string {
	return c.GetString(userIDKey)
}

// Token returns the raw session token of the current request, set by Auth.
func Token(c *gin.Context) string {
	return c.GetString(tokenKey)
}
