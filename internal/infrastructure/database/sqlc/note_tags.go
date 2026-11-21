package sqlc

import (
	"context"
	"fmt"
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

func (q *Queries) GetNoteTag(ctx context.Context, filter map[string]interface{}) ([]NoteTag, error) {
	query := `-- name: GetNoteTag :many
				SELECT id, notes_id, title
				FROM note_tags
				WHERE 1=1`

	var args []interface{}
	if noteID, ok := filter["notes_id"].(string); ok && noteID != "" {
		args = append(args, noteID)
		query += fmt.Sprintf(` AND notes_id = $%d`, len(args))
	} else if noteID, ok := filter["note_id"].(string); ok && noteID != "" {
		args = append(args, noteID)
		query += fmt.Sprintf(` AND notes_id = $%d`, len(args))
	}

	if title, ok := filter["title"].(string); ok && title != "" {
		args = append(args, title)
		query += fmt.Sprintf(` AND title = $%d`, len(args))
	}

	query += ` ORDER BY title ASC`

	if limit, ok := filter["limit"].(int); ok && limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	if offset, ok := filter["offset"].(int); ok && offset > 0 {
		args = append(args, offset)
		query += fmt.Sprintf(` OFFSET $%d`, len(args))
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var n []NoteTag
	for rows.Next() {
		var v NoteTag
		if err := rows.Scan(&v.ID, &v.NotesID, &v.Title); err != nil {
			return nil, err
		}
		n = append(n, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return n, nil
}

func (q *Queries) CountNoteTag(ctx context.Context, filter map[string]interface{}) (int, error) {
	query := `-- name: CountNoteTag :one
				SELECT COUNT(id)
				FROM note_tags
				WHERE 1=1`

	var args []interface{}
	if noteID, ok := filter["notes_id"].(string); ok && noteID != "" {
		args = append(args, noteID)
		query += fmt.Sprintf(` AND notes_id = $%d`, len(args))
	} else if noteID, ok := filter["note_id"].(string); ok && noteID != "" {
		args = append(args, noteID)
		query += fmt.Sprintf(` AND notes_id = $%d`, len(args))
	}

	if title, ok := filter["title"].(string); ok && title != "" {
		args = append(args, title)
		query += fmt.Sprintf(` AND title = $%d`, len(args))
	}

	var n int
	if err := q.db.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}

	return n, nil
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
