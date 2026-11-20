package auth

import (
	"github.com/golang-jwt/jwt/v4"
	"testing"
	"time"
)

func TestJWTRequiresCompleteUnexpiredHS256Identity(t *testing.T) {
	for _, test := range []struct {
		name          string
		method        jwt.SigningMethod
		id, nameClaim string
		expiry        *jwt.NumericDate
	}{
		{"missing-id", jwt.SigningMethodHS256, "", "alice", jwt.NewNumericDate(time.Now().Add(time.Hour))},
		{"missing-name", jwt.SigningMethodHS256, "user", "", jwt.NewNumericDate(time.Now().Add(time.Hour))},
		{"missing-expiry", jwt.SigningMethodHS256, "user", "alice", nil},
		{"expired", jwt.SigningMethodHS256, "user", "alice", jwt.NewNumericDate(time.Now().Add(-time.Hour))},
		{"wrong-algorithm", jwt.SigningMethodHS512, "user", "alice", jwt.NewNumericDate(time.Now().Add(time.Hour))},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := Claims{UserID: test.id, Username: test.nameClaim, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: test.expiry}}
			value, err := jwt.NewWithClaims(test.method, claims).SignedString(JWTSecret)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ValidateToken(value); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
}
