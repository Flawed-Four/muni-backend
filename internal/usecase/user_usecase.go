package usecase

import (
	"context"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type UserUsecase struct {
	users domain.UserRepository
}

func NewUserUsecase(users domain.UserRepository) *UserUsecase {
	return &UserUsecase{users: users}
}

func (u *UserUsecase) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return u.users.GetByID(ctx, id)
}
