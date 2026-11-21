package domain

import (
	"context"
	"time"
)

type Note struct {
	ID        string
	UserID    string
	Title     string
	Content   string
	Tags      []*NoteTag
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NoteFilter struct {
	UserID string
	Search string
	Limit  int
	Offset int
}

type NoteRepository interface {
	Create(ctx context.Context, note *Note) (*Note, error)
	GetByID(ctx context.Context, id string) (*Note, error)
	List(ctx context.Context, filter NoteFilter) ([]*Note, int, error)
	Update(ctx context.Context, note *Note) (*Note, error)
	Delete(ctx context.Context, id string) error
}
