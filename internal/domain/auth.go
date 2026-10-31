package domain

// TokenService issues and verifies access tokens.
type TokenService interface {
	Generate(userID string) (token string, err error)
	// Verify returns the user ID embedded in the token, or ErrInvalidToken.
	Verify(token string) (userID string, err error)
}

// PasswordHasher hashes passwords and compares them with stored hashes.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}
