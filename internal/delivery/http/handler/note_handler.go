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

type NoteHandler struct {
	notes *usecase.NoteUsecase
}

func NewNoteHandler(notes *usecase.NoteUsecase) *NoteHandler {
	return &NoteHandler{notes: notes}
}

// Create godoc
//
//	@Summary	Create a new note
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.CreateNoteRequest	true	"Note payload"
//	@Success	201		{object}	dto.NoteEnvelope
//	@Failure	400		{object}	response.Error
//	@Failure	401		{object}	response.Error
//	@Router		/notes [post]
func (h *NoteHandler) Create(c *gin.Context) {
	var req dto.CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, err.Error())
		return
	}

	note, err := h.notes.Create(c.Request.Context(), middleware.UserID(c), req.Title, req.Content)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, http.StatusCreated, dto.NewNoteResponse(note))
}

// GetByID godoc
//
//	@Summary	Get a note by ID
//	@Tags		notes
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Note ID"
//	@Success	200	{object}	dto.NoteEnvelope
//	@Failure	401	{object}	response.Error
//	@Failure	404	{object}	response.Error
//	@Router		/notes/{id} [get]
func (h *NoteHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	note, err := h.notes.GetByID(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, http.StatusOK, dto.NewNoteResponse(note))
}

// List godoc
//
//	@Summary	List notes
//	@Tags		notes
//	@Produce	json
//	@Security	BearerAuth
//	@Param		search	query		string	false	"Search keyword"
//	@Param		page	query		int		false	"Page number (default: 1)"
//	@Param		limit	query		int		false	"Items per page (default: 10)"
//	@Param		offset	query		int		false	"Offset (overrides page if set)"
//	@Success	200		{object}	dto.NoteListEnvelope
//	@Failure	401		{object}	response.Error
//	@Router		/notes [get]
func (h *NoteHandler) List(c *gin.Context) {
	search := c.Query("search")
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
			page = (offset / limit) + 1
		}
	}

	filter := domain.NoteFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	}

	notes, total, err := h.notes.List(c.Request.Context(), middleware.UserID(c), filter)
	if err != nil {
		response.Fail(c, err)
		return
	}

	items := make([]dto.NoteResponse, len(notes))
	for i, n := range notes {
		items[i] = dto.NewNoteResponse(n)
	}

	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	response.OK(c, http.StatusOK, dto.NoteListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Update godoc
//
//	@Summary	Update a note
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"Note ID"
//	@Param		body	body		dto.UpdateNoteRequest	true	"Update payload"
//	@Success	200		{object}	dto.NoteEnvelope
//	@Failure	400		{object}	response.Error
//	@Failure	401		{object}	response.Error
//	@Failure	404		{object}	response.Error
//	@Router		/notes/{id} [put]
func (h *NoteHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req dto.UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, err.Error())
		return
	}

	note, err := h.notes.Update(c.Request.Context(), middleware.UserID(c), id, req.Title, req.Content)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, http.StatusOK, dto.NewNoteResponse(note))
}

// Delete godoc
//
//	@Summary	Delete a note
//	@Tags		notes
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Note ID"
//	@Success	204
//	@Failure	401	{object}	response.Error
//	@Failure	404	{object}	response.Error
//	@Router		/notes/{id} [delete]
func (h *NoteHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.notes.Delete(c.Request.Context(), middleware.UserID(c), id); err != nil {
		response.Fail(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListTags godoc
//
//	@Summary	List tags for a note
//	@Tags		notes
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Note ID"
//	@Success	200	{object}	dto.NoteTagListEnvelope
//	@Failure	401	{object}	response.Error
//	@Failure	404	{object}	response.Error
//	@Router		/notes/{id}/tags [get]
func (h *NoteHandler) ListTags(c *gin.Context) {
	id := c.Param("id")
	tags, err := h.notes.ListTags(c.Request.Context(), middleware.UserID(c), id)
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
