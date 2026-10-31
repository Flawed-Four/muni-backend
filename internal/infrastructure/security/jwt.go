package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
)

type JWTService struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTService(cfg *config.Config) *JWTService {
	return &JWTService{secret: []byte(cfg.JWT.Secret), ttl: cfg.JWT.TTL}
}

func (s *JWTService) Generate(userID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTService) Verify(tokenStr string) (string, error) {
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(*jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", domain.ErrInvalidToken
	}
	return claims.Subject, nil
}
