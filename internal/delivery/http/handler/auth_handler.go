package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/moni-be/internal/delivery/http/dto"
	"github.com/Flawed-Four/moni-be/internal/delivery/http/response"
	"github.com/Flawed-Four/moni-be/internal/usecase"
)

type AuthHandler struct {
	auth *usecase.AuthUsecase
}

func NewAuthHandler(auth *usecase.AuthUsecase) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Register godoc
//
//	@Summary	Register a new user
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dto.RegisterRequest	true	"Registration payload"
//	@Success	201		{object}	dto.UserEnvelope
//	@Failure	400		{object}	response.Error
//	@Failure	409		{object}	response.Error
//	@Router		/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.auth.Register(c.Request.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusCreated, dto.NewUserResponse(user))
}

// Login godoc
//
//	@Summary	Log in and obtain an access token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dto.LoginRequest	true	"Login payload"
//	@Success	200		{object}	dto.TokenEnvelope
//	@Failure	400		{object}	response.Error
//	@Failure	401		{object}	response.Error
//	@Router		/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, err.Error())
		return
	}

	token, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, dto.TokenResponse{AccessToken: token, TokenType: "Bearer"})
}
