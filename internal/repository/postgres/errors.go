package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

// mapErr turns DB errors that every table shares into domain errors.
// Any other error is wrapped with op, so the request log shows which query failed.
func mapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	// An id that is not a valid UUID can never match a row.
	if errors.Is(err, pgx.ErrNoRows) || pgCode(err) == pgerrcode.InvalidTextRepresentation {
		return domain.ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}

// pgCode returns the Postgres error code of err, or "" if it has none.
func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
