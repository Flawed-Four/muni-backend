package domain

import "errors"

var (
	ErrNotFound           = errors.New("resource not found")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrPasswordTooLong    = errors.New("password is too long (max 72 bytes)")
	ErrDuplicateTag       = errors.New("tag already exists for this note")
	ErrForbidden          = errors.New("forbidden")
)
