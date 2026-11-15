package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database/sqlc"
)

type NoteRepository struct {
	q *sqlc.Queries
}

func NewNoteRepository(pool *pgxpool.Pool) *NoteRepository {
	return &NoteRepository{
		q: sqlc.New(pool),
	}
}

func (r *NoteRepository) Create(ctx context.Context, note *domain.Note) (*domain.Note, error) {
	row, err := r.q.CreateNote(ctx, sqlc.CreateNoteParams{
		UserID:    note.UserID,
		Title:     note.Title,
		Content:   note.Content,
		UpdatedAt: note.UpdatedAt,
	})
	if err != nil {
		return nil, mapNoteErr(err)
	}
	return toDomainNote(row), nil
}

func (r *NoteRepository) GetByID(ctx context.Context, id string) (*domain.Note, error) {
	row, err := r.q.GetNoteByID(ctx, id)
	if err != nil {
		return nil, mapNoteErr(err)
	}
	return toDomainNote(row), nil
}

func (r *NoteRepository) List(ctx context.Context, filter domain.NoteFilter) ([]*domain.Note, int, error) {
	filterMap := map[string]interface{}{}
	if filter.UserID != "" {
		filterMap["user_id"] = filter.UserID
	}
	if filter.Search != "" {
		filterMap["search"] = filter.Search
	}
	if filter.Limit > 0 {
		filterMap["limit"] = filter.Limit
	}
	if filter.Offset >= 0 {
		filterMap["offset"] = filter.Offset
	}

	rows, err := r.q.GetNotes(ctx, filterMap)
	if err != nil {
		return nil, 0, mapNoteErr(err)
	}

	total, err := r.q.CountNotes(ctx, filterMap)
	if err != nil {
		return nil, 0, mapNoteErr(err)
	}

	notes := make([]*domain.Note, len(rows))
	for i := range rows {
		notes[i] = toDomainNote(&rows[i])
	}

	return notes, total, nil
}

func (r *NoteRepository) Update(ctx context.Context, note *domain.Note) (*domain.Note, error) {
	row, err := r.q.UpdateNote(ctx, sqlc.UpdateNoteParams{
		ID:        note.ID,
		Title:     note.Title,
		Content:   note.Content,
		UpdatedAt: note.UpdatedAt,
	})
	if err != nil {
		return nil, mapNoteErr(err)
	}
	return toDomainNote(row), nil
}

func (r *NoteRepository) Delete(ctx context.Context, id string) error {
	if err := r.q.DeleteNote(ctx, id); err != nil {
		return mapNoteErr(err)
	}
	return nil
}

func mapNoteErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgerrcode.InvalidTextRepresentation, pgerrcode.ForeignKeyViolation:
			return domain.ErrNotFound
		}
	}
	return err
}

func toDomainNote(row *sqlc.Note) *domain.Note {
	return &domain.Note{
		ID:        row.ID,
		UserID:    row.UserID,
		Title:     row.Title,
		Content:   row.Content,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
