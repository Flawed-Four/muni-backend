package sqlc

import (
	"context"
	"fmt"
	"time"
)

type CreateNoteParams struct {
	UserID    string
	Title     string
	Content   string
	UpdatedAt time.Time
}

type UpdateNoteParams struct {
	ID        string
	Title     string
	Content   string
	UpdatedAt time.Time
}

func (q *Queries) CreateNote(ctx context.Context, arg CreateNoteParams) (*Note, error) {
	query := `-- name: CreateNote :one
				INSERT INTO notes (user_id, title, content, updated_at)
				VALUES ($1, $2, $3, $4)
				RETURNING id, user_id, title, content, created_at, updated_at`
	if arg.UpdatedAt.IsZero() {
		arg.UpdatedAt = time.Now()
	}
	row := q.db.QueryRow(ctx, query, arg.UserID, arg.Title, arg.Content, arg.UpdatedAt)

	var n Note
	if err := row.Scan(&n.ID, &n.UserID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}

	return &n, nil
}

func (q *Queries) GetNotes(ctx context.Context, filter map[string]interface{}) ([]Note, error) {
	query := `-- name: GetNotes :many
				SELECT id, user_id, title, content, created_at, updated_at
				FROM notes
				WHERE 1=1`

	var args []interface{}
	if userID, ok := filter["user_id"].(string); ok && userID != "" {
		args = append(args, userID)
		query += fmt.Sprintf(` AND user_id = $%d`, len(args))
	}

	if search, ok := filter["search"].(string); ok && search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(` AND (title ILIKE $%d OR content ILIKE $%d)`, len(args), len(args))
	}

	query += ` ORDER BY updated_at DESC`

	if limit, ok := filter["limit"].(int); ok && limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	if offset, ok := filter["offset"].(int); ok && offset >= 0 {
		args = append(args, offset)
		query += fmt.Sprintf(` OFFSET $%d`, len(args))
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var v Note
		if err := rows.Scan(&v.ID, &v.UserID, &v.Title, &v.Content, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notes, nil
}

func (q *Queries) CountNotes(ctx context.Context, filter map[string]interface{}) (int, error) {
	query := `-- name: CountNotes :one
				SELECT COUNT(id)
				FROM notes
				WHERE 1=1`

	var args []interface{}
	if userID, ok := filter["user_id"].(string); ok && userID != "" {
		args = append(args, userID)
		query += fmt.Sprintf(` AND user_id = $%d`, len(args))
	}

	if search, ok := filter["search"].(string); ok && search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(` AND (title ILIKE $%d OR content ILIKE $%d)`, len(args), len(args))
	}

	var n int
	if err := q.db.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}

	return n, nil
}

func (q *Queries) GetNoteByID(ctx context.Context, id string) (*Note, error) {
	query := `-- name: GetNoteByID :one
				SELECT id, user_id, title, content, created_at, updated_at
				FROM notes
				WHERE id = $1`

	row := q.db.QueryRow(ctx, query, id)
	var n Note
	if err := row.Scan(&n.ID, &n.UserID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}

	return &n, nil
}

func (q *Queries) UpdateNote(ctx context.Context, arg UpdateNoteParams) (*Note, error) {
	query := `-- name: UpdateNote :one
				UPDATE notes
				SET title = $1, content = $2, updated_at = $3
				WHERE id = $4
				RETURNING id, user_id, title, content, created_at, updated_at`

	if arg.UpdatedAt.IsZero() {
		arg.UpdatedAt = time.Now()
	}

	row := q.db.QueryRow(ctx, query, arg.Title, arg.Content, arg.UpdatedAt, arg.ID)
	var n Note
	if err := row.Scan(&n.ID, &n.UserID, &n.Title, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}

	return &n, nil
}

func (q *Queries) DeleteNote(ctx context.Context, id string) error {
	query := `-- name: DeleteNote :exec
				DELETE FROM notes
				WHERE id = $1`

	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return err
	}

	return nil
}
