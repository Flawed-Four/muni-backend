package usecase

import (
	"context"
	"time"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type NoteUsecase struct {
	notes domain.NoteRepository
	tags  domain.NoteTagRepository
}

func NewNoteUsecase(notes domain.NoteRepository, tags domain.NoteTagRepository) *NoteUsecase {
	return &NoteUsecase{
		notes: notes,
		tags:  tags,
	}
}

func (u *NoteUsecase) Create(ctx context.Context, userID, title, content string) (*domain.Note, error) {
	note, err := u.notes.Create(ctx, &domain.Note{
		UserID:    userID,
		Title:     title,
		Content:   content,
		UpdatedAt: time.Now(),
	})
	if err != nil {
		return nil, err
	}
	note.Tags = []*domain.NoteTag{}
	return note, nil
}

func (u *NoteUsecase) GetByID(ctx context.Context, userID, id string) (*domain.Note, error) {
	note, err := u.notes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if note.UserID != userID {
		return nil, domain.ErrNotFound
	}

	noteTags, err := u.tags.ListByNoteID(ctx, id)
	if err != nil {
		return nil, err
	}
	if noteTags == nil {
		noteTags = []*domain.NoteTag{}
	}
	note.Tags = noteTags

	return note, nil
}

func (u *NoteUsecase) List(ctx context.Context, userID string, filter domain.NoteFilter) ([]*domain.Note, int, error) {
	filter.UserID = userID
	notes, total, err := u.notes.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	if len(notes) > 0 {
		noteIDs := make([]string, len(notes))
		for i, n := range notes {
			noteIDs[i] = n.ID
		}

		tagsMap, err := u.tags.ListByNoteIDs(ctx, noteIDs)
		if err != nil {
			return nil, 0, err
		}
		for _, n := range notes {
			if tags, ok := tagsMap[n.ID]; ok {
				n.Tags = tags
			} else {
				n.Tags = []*domain.NoteTag{}
			}
		}
	}

	return notes, total, nil
}

func (u *NoteUsecase) Update(ctx context.Context, userID, id, title, content string) (*domain.Note, error) {
	note, err := u.notes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if note.UserID != userID {
		return nil, domain.ErrNotFound
	}

	note.Title = title
	note.Content = content
	note.UpdatedAt = time.Now()

	updatedNote, err := u.notes.Update(ctx, note)
	if err != nil {
		return nil, err
	}

	tags, err := u.tags.ListByNoteID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []*domain.NoteTag{}
	}
	updatedNote.Tags = tags

	return updatedNote, nil
}

func (u *NoteUsecase) Delete(ctx context.Context, userID, id string) error {
	note, err := u.notes.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if note.UserID != userID {
		return domain.ErrNotFound
	}

	return u.notes.Delete(ctx, id)
}

func (u *NoteUsecase) ListTags(ctx context.Context, userID, noteID string) ([]*domain.NoteTag, error) {
	note, err := u.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, err
	}
	if note.UserID != userID {
		return nil, domain.ErrNotFound
	}

	return u.tags.ListByNoteID(ctx, noteID)
}
