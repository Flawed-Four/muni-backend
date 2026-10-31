package domain

import (
	"context"
	"time"
)

// Role is the title of a row in user_roles. The constants must match the
// rows seeded by the migrations.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type User struct {
	ID           string
	Role         Role
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserRepository returns ErrNotFound when a user does not exist and
// ErrEmailTaken when Create violates the unique email constraint. Create
// stores the user under the role named by user.Role.
type UserRepository interface {
	Create(ctx context.Context, user *User) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
}
