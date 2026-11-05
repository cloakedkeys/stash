package user

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stashapp/stash/internal/manager/config"
)

var ErrInvalidToken = errors.New("invalid apikey")

const APIKeySubject = "APIKey"

type APIKeyClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

// apiKeyIDBytes is the number of random bytes in each key's ID.
const apiKeyIDBytes = 16

func generateAPIKey(userID string) (string, error) {
	// The user name and the time in whole seconds are the same for keys made
	// in the same second, so each key also gets a random ID. Without it,
	// replacing a key twice quickly gives back the same key, and the key that
	// was meant to be replaced keeps working.
	id := make([]byte, apiKeyIDBytes)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("generating api key id: %w", err)
	}

	claims := &APIKeyClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:       hex.EncodeToString(id),
			Subject:  APIKeySubject,
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	ss, err := token.SignedString(config.GetInstance().GetJWTSignKey())
	if err != nil {
		return "", err
	}

	return ss, nil
}

// GetUserIDFromAPIKey validates the provided api key and returns the user ID
func GetUserIDFromAPIKey(apiKey string) (string, error) {
	claims := &APIKeyClaims{}
	token, err := jwt.ParseWithClaims(apiKey, claims, func(t *jwt.Token) (interface{}, error) {
		return config.GetInstance().GetJWTSignKey(), nil
	})

	if err != nil {
		return "", ErrInvalidToken
	}

	if !token.Valid {
		return "", ErrInvalidToken
	}

	return claims.UserID, nil
}
