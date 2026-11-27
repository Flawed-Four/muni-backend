package security

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type BcryptHasher struct{}

func NewBcryptHasher() *BcryptHasher {
	return &BcryptHasher{}
}

// Hash rejects passwords over 72 bytes, the most bcrypt can use. A limit in
// characters is not enough: "é" is 1 character but 2 bytes.
func (BcryptHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return "", domain.ErrPasswordTooLong
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (BcryptHasher) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
