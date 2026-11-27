package postgres

import (
	"context"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/database/sqlc"
)

type NoteTagRepository struct {
	q *sqlc.Queries
}

func NewNoteTagRepository(pool *pgxpool.Pool) *NoteTagRepository {
	return &NoteTagRepository{
		q: sqlc.New(pool),
	}
}

func (r *NoteTagRepository) Create(ctx context.Context, tag *domain.NoteTag) (*domain.NoteTag, error) {
	row, err := r.q.CreateNoteTag(ctx, sqlc.CreateNoteTagParams{
		NoteID: tag.NoteID,
		Title:  tag.Title,
	})
	if err != nil {
		return nil, mapNoteTagErr(err, "create note tag")
	}
	return toDomainNoteTag(row), nil
}

func (r *NoteTagRepository) GetByID(ctx context.Context, id string) (*domain.NoteTag, error) {
	row, err := r.q.GetNoteTagByID(ctx, id)
	if err != nil {
		return nil, mapNoteTagErr(err, "get note tag by id")
	}
	return toDomainNoteTag(row), nil
}

func (r *NoteTagRepository) ListByNoteID(ctx context.Context, noteID string) ([]*domain.NoteTag, error) {
	rows, err := r.q.GetNoteTagsByNoteID(ctx, noteID)
	if err != nil {
		return nil, mapNoteTagErr(err, "list note tags by note id")
	}
	tags := make([]*domain.NoteTag, len(rows))
	for i := range rows {
		tags[i] = toDomainNoteTag(&rows[i])
	}
	return tags, nil
}

func (r *NoteTagRepository) ListByNoteIDs(ctx context.Context, noteIDs []string) (map[string][]*domain.NoteTag, error) {
	result := make(map[string][]*domain.NoteTag)
	if len(noteIDs) == 0 {
		return result, nil
	}

	rows, err := r.q.GetNoteTagsByNoteIDs(ctx, noteIDs)
	if err != nil {
		return nil, mapNoteTagErr(err, "list note tags by note ids")
	}

	for i := range rows {
		tag := toDomainNoteTag(&rows[i])
		result[tag.NoteID] = append(result[tag.NoteID], tag)
	}

	return result, nil
}

func (r *NoteTagRepository) List(ctx context.Context, filter domain.NoteTagFilter) ([]*domain.NoteTag, int, error) {
	var noteID pgtype.UUID
	if filter.NoteID != "" {
		// An id that is not a valid UUID can never match a row.
		if err := noteID.Scan(filter.NoteID); err != nil {
			return nil, 0, domain.ErrNotFound
		}
	}
	title := pgtype.Text{String: filter.Title, Valid: filter.Title != ""}

	rows, err := r.q.ListUserNoteTags(ctx, sqlc.ListUserNoteTagsParams{
		UserID:    filter.UserID,
		NoteID:    noteID,
		Title:     title,
		RowLimit:  int64(filter.Limit),
		RowOffset: int64(max(filter.Offset, 0)),
	})
	if err != nil {
		return nil, 0, mapNoteTagErr(err, "list note tags")
	}

	total, err := r.q.CountUserNoteTags(ctx, sqlc.CountUserNoteTagsParams{
		UserID: filter.UserID,
		NoteID: noteID,
		Title:  title,
	})
	if err != nil {
		return nil, 0, mapNoteTagErr(err, "count note tags")
	}

	tags := make([]*domain.NoteTag, len(rows))
	for i := range rows {
		tags[i] = toDomainNoteTag(&rows[i].NoteTag)
	}

	return tags, int(total), nil
}

func (r *NoteTagRepository) Update(ctx context.Context, id string, title string) (*domain.NoteTag, error) {
	if err := r.q.UpdateNoteTag(ctx, title, id); err != nil {
		return nil, mapNoteTagErr(err, "update note tag")
	}
	return r.GetByID(ctx, id)
}

func (r *NoteTagRepository) Delete(ctx context.Context, id string) error {
	if err := r.q.DeleteNoteTag(ctx, id); err != nil {
		return mapNoteTagErr(err, "delete note tag")
	}
	return nil
}

func (r *NoteTagRepository) DeleteByNoteID(ctx context.Context, noteID string) error {
	if err := r.q.DeleteNoteTagsByNoteID(ctx, noteID); err != nil {
		return mapNoteTagErr(err, "delete note tags by note id")
	}
	return nil
}

func mapNoteTagErr(err error, op string) error {
	switch pgCode(err) {
	case pgerrcode.UniqueViolation:
		return domain.ErrDuplicateTag
	case pgerrcode.ForeignKeyViolation:
		return domain.ErrNotFound
	}
	return mapErr(err, op)
}

func toDomainNoteTag(row *sqlc.NoteTag) *domain.NoteTag {
	return &domain.NoteTag{
		ID:     row.ID,
		NoteID: row.NotesID,
		Title:  row.Title,
	}
}
