package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/moni-be/internal/delivery/http/dto"
	"github.com/Flawed-Four/moni-be/internal/delivery/http/response"
)

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db Pinger
}

func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Check godoc
//
//	@Summary	Liveness and database readiness check
//	@Tags		health
//	@Produce	json
//	@Success	200	{object}	dto.HealthResponse
//	@Failure	503	{object}	response.Error
//	@Router		/health [get]
func (h *HealthHandler) Check(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		response.Abort(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	c.JSON(http.StatusOK, dto.HealthResponse{Status: "ok"})
}
