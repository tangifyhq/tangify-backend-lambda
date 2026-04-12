package main

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type MyClaims struct {
	Identity string `json:"identity"` // user id
	Name     string `json:"name"`
	Role     string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

type JwtUtils struct {
	secret []byte
}

var jwtUtils *JwtUtils

func NewJwtUtils(secret string) *JwtUtils {
	if jwtUtils == nil {
		jwtUtils = &JwtUtils{
			secret: []byte(secret),
		}
		return jwtUtils
	}
	return jwtUtils
}

// ParseJWT parses a JWT token string using the given secret (HS256)
// and returns the custom claims
func (j *JwtUtils) ParseJWT(tokenString string) (*MyClaims, error) {
	claims := &MyClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return j.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// GenerateJWT signs an access token (HS256) with subject = userID.
func (j *JwtUtils) GenerateJWT(userID, name, role string, ttl time.Duration) (string, error) {
	claims := MyClaims{
		Identity: userID,
		Name:     name,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:    jwt.NewNumericDate(time.Now()),
			Subject:     userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}
