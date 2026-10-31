package domain

// TokenService generates opaque session tokens and hashes them for storage.
// Only the hash is ever persisted; the raw token is handed to the client once.
type TokenService interface {
	// New returns a fresh random token and the hash to store for it.
	New() (token string, hash []byte, err error)
	// Hash returns the storage hash for a token presented by a client.
	Hash(token string) []byte
}

// PasswordHasher hashes passwords and compares them with stored hashes.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}
