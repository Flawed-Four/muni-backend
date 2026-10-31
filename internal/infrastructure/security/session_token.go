package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const tokenBytes = 32

// SessionTokens issues 256-bit random tokens. They carry no data, so a
// plain SHA-256 is enough to protect them at rest (no salt or bcrypt needed).
type SessionTokens struct{}

func NewSessionTokens() *SessionTokens {
	return &SessionTokens{}
}

func (SessionTokens) New() (string, []byte, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, SessionTokens{}.Hash(token), nil
}

func (SessionTokens) Hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
