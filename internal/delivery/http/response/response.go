package response

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type Success struct {
	Data any `json:"data"`
}

type ErrorBody struct {
	Message string `json:"message"`
}

type Error struct {
	Error ErrorBody `json:"error"`
}

func OK(c *gin.Context, status int, data any) {
	c.JSON(status, Success{Data: data})
}

func Abort(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, Error{Error: ErrorBody{Message: message}})
}

// Fail maps a domain error to an HTTP status. Unknown errors are logged and
// reported as 500 without leaking internals.
func Fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		Abort(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrDuplicateTag):
		Abort(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken):
		Abort(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		Abort(c, http.StatusForbidden, err.Error())
	default:
		slog.Error("internal error", "path", c.FullPath(), "err", err)
		Abort(c, http.StatusInternalServerError, "internal server error")
	}
}
