package domain

import (
	"context"
)

type NoteTag struct {
	ID     string
	NoteID string
	Tags   []string
	Title  string
}

type NoteTagFilter struct {
	UserID string // required: only tags on this user's notes are listed
	NoteID string
	Title  string
	Limit  int
	Offset int
}

type NoteTagRepository interface {
	Create(ctx context.Context, tag *NoteTag) (*NoteTag, error)
	GetByID(ctx context.Context, id string) (*NoteTag, error)
	ListByNoteID(ctx context.Context, noteID string) ([]*NoteTag, error)
	ListByNoteIDs(ctx context.Context, noteIDs []string) (map[string][]*NoteTag, error)
	List(ctx context.Context, filter NoteTagFilter) ([]*NoteTag, int, error)
	Update(ctx context.Context, id string, title string) (*NoteTag, error)
	Delete(ctx context.Context, id string) error
	DeleteByNoteID(ctx context.Context, noteID string) error
}
