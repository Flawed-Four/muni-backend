package dto

import (
	"time"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type CreateNoteRequest struct {
	Title   string `json:"title" binding:"required,min=1,max=255" example:"My Meeting Notes"`
	Content string `json:"content" binding:"required" example:"Discussed the new roadmap..."`
}

type UpdateNoteRequest struct {
	Title   string `json:"title" binding:"required,min=1,max=255" example:"Updated Meeting Notes"`
	Content string `json:"content" binding:"required" example:"Discussed the new roadmap and timeline..."`
}

type NoteResponse struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Title     string            `json:"title"`
	Content   string            `json:"content"`
	Tags      []NoteTagResponse `json:"tags"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func NewNoteResponse(n *domain.Note) NoteResponse {
	tagResponses := make([]NoteTagResponse, 0, len(n.Tags))
	for _, t := range n.Tags {
		tagResponses = append(tagResponses, NewNoteTagResponse(t))
	}
	return NoteResponse{
		ID:        n.ID,
		UserID:    n.UserID,
		Title:     n.Title,
		Content:   n.Content,
		Tags:      tagResponses,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

type NoteListResponse struct {
	Items      []NoteResponse `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}

type NoteEnvelope struct {
	Data NoteResponse `json:"data"`
}

type NoteListEnvelope struct {
	Data NoteListResponse `json:"data"`
}
