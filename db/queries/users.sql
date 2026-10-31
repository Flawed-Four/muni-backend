-- name: CreateUser :one
-- The role is resolved by title; zero rows are returned (pgx.ErrNoRows) if it does not exist.
INSERT INTO users (role_id, name, email, password_hash)
SELECT r.id, sqlc.arg(name), sqlc.arg(email), sqlc.arg(password_hash)
FROM user_roles r
WHERE r.title = sqlc.arg(role)
RETURNING *;

-- name: GetUserByID :one
SELECT sqlc.embed(users), user_roles.title AS role
FROM users
JOIN user_roles ON user_roles.id = users.role_id
WHERE users.id = $1;

-- name: GetUserByEmail :one
SELECT sqlc.embed(users), user_roles.title AS role
FROM users
JOIN user_roles ON user_roles.id = users.role_id
WHERE users.email = $1;
