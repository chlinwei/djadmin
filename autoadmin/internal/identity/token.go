package identity

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const ClaimsContextKey = "identity.claims"

type Claims struct {
	UserID   int32    `json:"user_id"`
	Username string   `json:"username"`
	Perms    []string `json:"perms"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret     []byte
	expiration time.Duration
}

func NewTokenManager(secret string, expiration time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), expiration: expiration}
}

func (manager *TokenManager) Issue(userID int32, username string, permissions []string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Perms:    permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(manager.expiration)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(manager.secret)
}

func (manager *TokenManager) Parse(rawToken string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected JWT signing method %s", token.Method.Alg())
		}
		return manager.secret, nil
	}, jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("validate JWT: %w", err)
	}
	return claims, nil
}

func ClaimsFromContext(context *gin.Context) (*Claims, bool) {
	value, exists := context.Get(ClaimsContextKey)
	claims, ok := value.(*Claims)
	return claims, exists && ok
}

func IsTokenExpired(err error) bool {
	return errors.Is(err, jwt.ErrTokenExpired)
}
