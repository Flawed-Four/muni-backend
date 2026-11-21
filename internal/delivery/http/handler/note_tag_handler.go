package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Flawed-Four/muni-backend/internal/delivery/http/dto"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/middleware"
	"github.com/Flawed-Four/muni-backend/internal/delivery/http/response"
	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/usecase"
)

type NoteTagHandler struct {
	tags *usecase.NoteTagUsecase
}

func NewNoteTagHandler(tags *usecase.NoteTagUsecase) *NoteTagHandler {
	return &NoteTagHandler{tags: tags}
}

// Create godoc
//
//	@Summary	Add a tag to a note
//	@Tags		tags
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.CreateNoteTagRequest	true	"Tag payload"
//	@Success	201		{object}	dto.NoteTagEnvelope
//	@Failure	400		{object}	response.Error
//	@Failure	401		{object}	response.Error
//	@Failure	404		{object}	response.Error
//	@Failure	409		{object}	response.Error
//	@Router		/tags [post]
func (h *NoteTagHandler) Create(c *gin.Context) {
	var req dto.CreateNoteTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.tags.Create(c.Request.Context(), middleware.UserID(c), req.NoteID, req.Title)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, http.StatusCreated, dto.NewNoteTagResponse(tag))
}

// GetByID godoc
//
//	@Summary	Get a tag by ID
//	@Tags		tags
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Tag ID"
//	@Success	200	{object}	dto.NoteTagEnvelope
//	@Failure	401	{object}	response.Error
//	@Failure	404	{object}	response.Error
//	@Router		/tags/{id} [get]
func (h *NoteTagHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tag, err := h.tags.GetByID(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, http.StatusOK, dto.NewNoteTagResponse(tag))
}

// List godoc
//
//	@Summary	List tags
//	@Tags		tags
//	@Produce	json
//	@Security	BearerAuth
//	@Param		note_id	query		string	false	"Note ID"
//	@Param		title	query		string	false	"Tag Title"
//	@Param		page	query		int		false	"Page number (default: 1)"
//	@Param		limit	query		int		false	"Items per page (default: 10)"
//	@Param		offset	query		int		false	"Offset"
//	@Success	200		{object}	dto.NoteTagListEnvelope
//	@Failure	401		{object}	response.Error
//	@Failure	404		{object}	response.Error
//	@Router		/tags [get]
func (h *NoteTagHandler) List(c *gin.Context) {
	noteID := c.Query("note_id")
	title := c.Query("title")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if o, err := strconv.Atoi(offsetParam); err == nil && o >= 0 {
			offset = o
		}
	}

	filter := domain.NoteTagFilter{
		NoteID: noteID,
		Title:  title,
		Limit:  limit,
		Offset: offset,
	}

	tags, _, err := h.tags.List(c.Request.Context(), middleware.UserID(c), filter)
	if err != nil {
		response.Fail(c, err)
		return
	}

	tagResponses := make([]dto.NoteTagResponse, len(tags))
	for i, t := range tags {
		tagResponses[i] = dto.NewNoteTagResponse(t)
	}

	response.OK(c, http.StatusOK, tagResponses)
}

// Update godoc
//
//	@Summary	Update a tag
//	@Tags		tags
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"Tag ID"
//	@Param		body	body		dto.UpdateNoteTagRequest	true	"Update payload"
//	@Success	200		{object}	dto.NoteTagEnvelope
//	@Failure	400		{object}	response.Error
//	@Failure	401		{object}	response.Error
//	@Failure	404		{object}	response.Error
//	@Failure	409		{object}	response.Error
//	@Router		/tags/{id} [put]
func (h *NoteTagHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateNoteTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.tags.Update(c.Request.Context(), middleware.UserID(c), id, req.Title)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, http.StatusOK, dto.NewNoteTagResponse(tag))
}

// Delete godoc
//
//	@Summary	Delete a tag
//	@Tags		tags
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Tag ID"
//	@Success	204
//	@Failure	401	{object}	response.Error
//	@Failure	404	{object}	response.Error
//	@Router		/tags/{id} [delete]
func (h *NoteTagHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.tags.Delete(c.Request.Context(), middleware.UserID(c), id); err != nil {
		response.Fail(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
