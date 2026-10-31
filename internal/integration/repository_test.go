package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/repository/postgres"
)

func newUser(email string) *domain.User {
	return &domain.User{Role: domain.RoleUser, Name: "Test User", Email: email, PasswordHash: "hash"}
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
	if created.Role != domain.RoleUser {
		t.Fatalf("created role = %q, want %q", created.Role, domain.RoleUser)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Email != "a@example.com" || byID.PasswordHash != "hash" || byID.Role != domain.RoleUser {
		t.Fatalf("GetByID returned %+v", byID)
	}

	byEmail, err := repo.GetByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != created.ID || byEmail.Role != domain.RoleUser {
		t.Fatalf("GetByEmail = %+v, want id %s with role %q", byEmail, created.ID, domain.RoleUser)
	}
}

func TestUserRepository_AdminRole(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewUserRepository(testPool)

	u := newUser("admin@example.com")
	u.Role = domain.RoleAdmin
	created, err := repo.Create(ctx, u)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Role != domain.RoleAdmin {
		t.Fatalf("role = %q, want %q", got.Role, domain.RoleAdmin)
	}
}

func TestUserRepository_UnknownRole(t *testing.T) {
	resetDB(t)
	repo := postgres.NewUserRepository(testPool)

	u := newUser("ghost@example.com")
	u.Role = "ghost"
	_, err := repo.Create(context.Background(), u)
	if err == nil {
		t.Fatal("Create with unknown role succeeded, want error")
	}
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("error = %v, want a plain internal error", err)
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
