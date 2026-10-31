package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/moni-be/internal/delivery/http/dto"
	"github.com/Flawed-Four/moni-be/internal/delivery/http/middleware"
	"github.com/Flawed-Four/moni-be/internal/delivery/http/response"
	"github.com/Flawed-Four/moni-be/internal/usecase"
)

type UserHandler struct {
	users *usecase.UserUsecase
}

func NewUserHandler(users *usecase.UserUsecase) *UserHandler {
	return &UserHandler{users: users}
}

// Me godoc
//
//	@Summary	Get the authenticated user's profile
//	@Tags		users
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	dto.UserEnvelope
//	@Failure	401	{object}	response.Error
//	@Failure	404	{object}	response.Error
//	@Router		/users/me [get]
func (h *UserHandler) Me(c *gin.Context) {
	user, err := h.users.GetByID(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, dto.NewUserResponse(user))
}
