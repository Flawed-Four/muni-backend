package dto

import (
	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type CreateNoteTagRequest struct {
	NoteID string `json:"note_id" binding:"required,uuid" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	Title  string `json:"title" binding:"required,min=1,max=255" example:"work"`
}

type UpdateNoteTagRequest struct {
	Title string `json:"title" binding:"required,min=1,max=255" example:"personal"`
}

type NoteTagResponse struct {
	ID     string `json:"id"`
	NoteID string `json:"note_id"`
	Title  string `json:"title"`
}

func NewNoteTagResponse(t *domain.NoteTag) NoteTagResponse {
	return NoteTagResponse{
		ID:     t.ID,
		NoteID: t.NoteID,
		Title:  t.Title,
	}
}

type NoteTagEnvelope struct {
	Data NoteTagResponse `json:"data"`
}

type NoteTagListEnvelope struct {
	Data []NoteTagResponse `json:"data"`
}
