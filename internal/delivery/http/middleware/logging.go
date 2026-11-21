package middleware

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/muni-backend/internal/delivery/http/response"
	"github.com/Flawed-Four/muni-backend/internal/logging"
)

// RequestIDHeader carries the request ID in both directions.
const RequestIDHeader = "X-Request-ID"

// RequestLogger gives each request an ID and writes one log line when it
// finishes. Errors attached with c.Error (see response.Fail) go into that line.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		id := c.GetHeader(RequestIDHeader)
		if !validRequestID(id) {
			id = rand.Text()
		}
		c.Header(RequestIDHeader, id)
		c.Request = c.Request.WithContext(logging.With(c.Request.Context(), slog.String("request_id", id)))

		c.Next()

		status := c.Writer.Status()
		route := c.FullPath() // the pattern, not the raw URL, so query strings never reach the log
		if route == "/health" && status < http.StatusInternalServerError {
			return
		}

		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("ip", c.ClientIP()),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("err", strings.Join(c.Errors.Errors(), "; ")))
		}
		slog.LogAttrs(c.Request.Context(), level, "request", attrs...)
	}
}

// Recovery turns a panic into a 500 and logs it with its stack trace.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, rec any) {
		slog.ErrorContext(c.Request.Context(), "panic recovered",
			slog.String("panic", fmt.Sprint(rec)), slog.String("stack", string(debug.Stack())))
		_ = c.Error(fmt.Errorf("panic: %v", rec))
		response.Abort(c, http.StatusInternalServerError, "internal server error")
	})
}

// validRequestID accepts an incoming ID only if it is short and plain,
// so a client cannot inject odd characters into the logs.
func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		b := id[i]
		ok := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.'
		if !ok {
			return false
		}
	}
	return true
}
