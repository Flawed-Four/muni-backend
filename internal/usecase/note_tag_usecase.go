package usecase

import (
	"context"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type NoteTagUsecase struct {
	notes domain.NoteRepository
	tags  domain.NoteTagRepository
}

func NewNoteTagUsecase(notes domain.NoteRepository, tags domain.NoteTagRepository) *NoteTagUsecase {
	return &NoteTagUsecase{
		notes: notes,
		tags:  tags,
	}
}

func (u *NoteTagUsecase) Create(ctx context.Context, userID, noteID, title string) (*domain.NoteTag, error) {
	note, err := u.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, err
	}
	if note.UserID != userID {
		return nil, domain.ErrNotFound
	}

	return u.tags.Create(ctx, &domain.NoteTag{
		NoteID: noteID,
		Title:  title,
	})
}

func (u *NoteTagUsecase) GetByID(ctx context.Context, userID, id string) (*domain.NoteTag, error) {
	tag, err := u.tags.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	note, err := u.notes.GetByID(ctx, tag.NoteID)
	if err != nil {
		return nil, err
	}
	if note.UserID != userID {
		return nil, domain.ErrNotFound
	}

	return tag, nil
}

func (u *NoteTagUsecase) Update(ctx context.Context, userID, id, title string) (*domain.NoteTag, error) {
	tag, err := u.tags.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	note, err := u.notes.GetByID(ctx, tag.NoteID)
	if err != nil {
		return nil, err
	}
	if note.UserID != userID {
		return nil, domain.ErrNotFound
	}

	return u.tags.Update(ctx, id, title)
}

func (u *NoteTagUsecase) Delete(ctx context.Context, userID, id string) error {
	tag, err := u.tags.GetByID(ctx, id)
	if err != nil {
		return err
	}

	note, err := u.notes.GetByID(ctx, tag.NoteID)
	if err != nil {
		return err
	}
	if note.UserID != userID {
		return domain.ErrNotFound
	}

	return u.tags.Delete(ctx, id)
}

func (u *NoteTagUsecase) List(ctx context.Context, userID string, filter domain.NoteTagFilter) ([]*domain.NoteTag, int, error) {
	if filter.NoteID != "" {
		note, err := u.notes.GetByID(ctx, filter.NoteID)
		if err != nil {
			return nil, 0, err
		}
		if note.UserID != userID {
			return nil, 0, domain.ErrNotFound
		}
	}

	return u.tags.List(ctx, filter)
}
