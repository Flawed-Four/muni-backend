package sqlc

import (
	"context"
)

type CreateNoteTagParams struct {
	NoteID string
	Title  string
}

func (q *Queries) CreateNoteTag(ctx context.Context, arg CreateNoteTagParams) (*NoteTag, error) {
	query := `-- name: CreateNoteTag :one
				INSERT INTO note_tags (notes_id, title)
				VALUES ($1, $2)
				RETURNING id, notes_id, title`
	row := q.db.QueryRow(ctx, query, arg.NoteID, arg.Title)

	var n NoteTag
	if err := row.Scan(&n.ID, &n.NotesID, &n.Title); err != nil {
		return nil, err
	}

	return &n, nil
}

func (q *Queries) GetNoteTagByID(ctx context.Context, id string) (*NoteTag, error) {
	query := `-- name: GetNoteTagByID :one
				SELECT id, notes_id, title
				FROM note_tags
				WHERE id = $1`

	row := q.db.QueryRow(ctx, query, id)
	var n NoteTag
	if err := row.Scan(&n.ID, &n.NotesID, &n.Title); err != nil {
		return nil, err
	}

	return &n, nil
}

func (q *Queries) GetNoteTagByNoteID(ctx context.Context, id string) (*NoteTag, error) {
	query := `-- name: GetNoteTagByNoteID :one
				SELECT id, notes_id, title
				FROM note_tags
				WHERE notes_id = $1
				LIMIT 1`

	row := q.db.QueryRow(ctx, query, id)
	var n NoteTag
	if err := row.Scan(&n.ID, &n.NotesID, &n.Title); err != nil {
		return nil, err
	}

	return &n, nil
}

func (q *Queries) GetNoteTagsByNoteID(ctx context.Context, noteID string) ([]NoteTag, error) {
	query := `-- name: GetNoteTagsByNoteID :many
				SELECT id, notes_id, title
				FROM note_tags
				WHERE notes_id = $1
				ORDER BY title ASC`

	rows, err := q.db.Query(ctx, query, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []NoteTag
	for rows.Next() {
		var v NoteTag
		if err := rows.Scan(&v.ID, &v.NotesID, &v.Title); err != nil {
			return nil, err
		}
		tags = append(tags, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tags, nil
}

func (q *Queries) GetNoteTagsByNoteIDs(ctx context.Context, noteIDs []string) ([]NoteTag, error) {
	if len(noteIDs) == 0 {
		return nil, nil
	}

	query := `-- name: GetNoteTagsByNoteIDs :many
				SELECT id, notes_id, title
				FROM note_tags
				WHERE notes_id::text = ANY($1)
				ORDER BY title ASC`

	rows, err := q.db.Query(ctx, query, noteIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []NoteTag
	for rows.Next() {
		var v NoteTag
		if err := rows.Scan(&v.ID, &v.NotesID, &v.Title); err != nil {
			return nil, err
		}
		tags = append(tags, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tags, nil
}

func (q *Queries) UpdateNoteTag(ctx context.Context, title string, id string) error {
	query := `-- name: UpdateNoteTag :exec
				UPDATE note_tags
				SET title = $1
				WHERE id = $2`

	if _, err := q.db.Exec(ctx, query, title, id); err != nil {
		return err
	}

	return nil
}

func (q *Queries) DeleteNoteTag(ctx context.Context, id string) error {
	query := `-- name: DeleteNoteTag :exec
				DELETE FROM note_tags
				WHERE id = $1`

	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return err
	}

	return nil
}

func (q *Queries) DeleteNoteTagsByNoteID(ctx context.Context, noteID string) error {
	query := `-- name: DeleteNoteTagsByNoteID :exec
				DELETE FROM note_tags
				WHERE notes_id = $1`

	if _, err := q.db.Exec(ctx, query, noteID); err != nil {
		return err
	}

	return nil
}
