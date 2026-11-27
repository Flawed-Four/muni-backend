-- name: ListUserNoteTags :many
-- Only tags on the user's own notes; note_id and title are optional filters.
SELECT sqlc.embed(note_tags)
FROM note_tags
JOIN notes ON notes.id = note_tags.notes_id
WHERE notes.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(note_id)::uuid IS NULL OR note_tags.notes_id = sqlc.narg(note_id))
  AND (sqlc.narg(title)::text IS NULL OR note_tags.title = sqlc.narg(title))
ORDER BY note_tags.title, note_tags.id
LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: CountUserNoteTags :one
SELECT COUNT(*)
FROM note_tags
JOIN notes ON notes.id = note_tags.notes_id
WHERE notes.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(note_id)::uuid IS NULL OR note_tags.notes_id = sqlc.narg(note_id))
  AND (sqlc.narg(title)::text IS NULL OR note_tags.title = sqlc.narg(title));
