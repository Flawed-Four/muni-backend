package integration

import (
	"context"
	"errors"
	"strings"
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

// The request log line only shows the error text, so an error that is not a
// domain sentinel must say which query failed.
func TestRepositoryErrorsNameTheOperation(t *testing.T) {
	resetDB(t)
	// A canceled context makes every query fail with an error no repository maps.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const id = "00000000-0000-0000-0000-000000000000"
	users := postgres.NewUserRepository(testPool)
	sessions := postgres.NewSessionRepository(testPool)
	notes := postgres.NewNoteRepository(testPool)
	tags := postgres.NewNoteTagRepository(testPool)

	// Each key is the operation the error must start with.
	cases := map[string]func() error{
		"create user": func() error {
			_, err := users.Create(ctx, newUser("a@example.com"))
			return err
		},
		"get user by id": func() error {
			_, err := users.GetByID(ctx, id)
			return err
		},
		"get user by email": func() error {
			_, err := users.GetByEmail(ctx, "a@example.com")
			return err
		},
		"create session": func() error {
			_, err := sessions.Create(ctx, &domain.Session{UserID: id})
			return err
		},
		"touch session": func() error {
			_, err := sessions.Touch(ctx, []byte("hash"))
			return err
		},
		"revoke session": func() error {
			return sessions.Revoke(ctx, []byte("hash"))
		},
		"revoke user sessions": func() error {
			return sessions.RevokeAllForUser(ctx, id)
		},
		"create note": func() error {
			_, err := notes.Create(ctx, &domain.Note{UserID: id, Title: "t"})
			return err
		},
		"get note by id": func() error {
			_, err := notes.GetByID(ctx, id)
			return err
		},
		"list notes": func() error {
			_, _, err := notes.List(ctx, domain.NoteFilter{UserID: id})
			return err
		},
		"update note": func() error {
			_, err := notes.Update(ctx, &domain.Note{ID: id, Title: "t"})
			return err
		},
		"delete note": func() error {
			return notes.Delete(ctx, id)
		},
		"create note tag": func() error {
			_, err := tags.Create(ctx, &domain.NoteTag{NoteID: id, Title: "t"})
			return err
		},
		"get note tag by id": func() error {
			_, err := tags.GetByID(ctx, id)
			return err
		},
		"list note tags by note id": func() error {
			_, err := tags.ListByNoteID(ctx, id)
			return err
		},
		"list note tags by note ids": func() error {
			_, err := tags.ListByNoteIDs(ctx, []string{id})
			return err
		},
		"list note tags": func() error {
			_, _, err := tags.List(ctx, domain.NoteTagFilter{NoteID: id})
			return err
		},
		"update note tag": func() error {
			_, err := tags.Update(ctx, id, "t")
			return err
		},
		"delete note tag": func() error {
			return tags.Delete(ctx, id)
		},
		"delete note tags by note id": func() error {
			return tags.DeleteByNoteID(ctx, id)
		},
	}
	for op, call := range cases {
		t.Run(op, func(t *testing.T) {
			err := call()
			if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), op+": ") {
				t.Fatalf("error = %v, want %q followed by context.Canceled", err, op+": ")
			}
		})
	}
}
