package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Flawed-Four/moni-be/internal/domain"
	"github.com/Flawed-Four/moni-be/internal/repository/postgres"
)

func newUser(email string) *domain.User {
	return &domain.User{Name: "Test User", Email: email, PasswordHash: "hash"}
}

func TestUserRepository_CreateAndGet(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewUserRepository(testPool)

	created, err := repo.Create(ctx, newUser("a@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() {
		t.Fatalf("expected DB-generated id and created_at, got %+v", created)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Email != "a@example.com" || byID.PasswordHash != "hash" {
		t.Fatalf("GetByID returned %+v", byID)
	}

	byEmail, err := repo.GetByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != created.ID {
		t.Fatalf("GetByEmail id = %s, want %s", byEmail.ID, created.ID)
	}
}

func TestUserRepository_DuplicateEmail(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewUserRepository(testPool)

	if _, err := repo.Create(ctx, newUser("dup@example.com")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := repo.Create(ctx, newUser("dup@example.com"))
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("second Create error = %v, want ErrEmailTaken", err)
	}
}

func TestUserRepository_NotFound(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewUserRepository(testPool)

	cases := map[string]func() error{
		"GetByID unknown uuid": func() error {
			_, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
			return err
		},
		"GetByID malformed uuid": func() error {
			_, err := repo.GetByID(ctx, "not-a-uuid")
			return err
		},
		"GetByEmail unknown": func() error {
			_, err := repo.GetByEmail(ctx, "nobody@example.com")
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
	}
}
