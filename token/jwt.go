package token

import (
	"crypto/rsa"
	"fmt"
	"io"

	"github.com/golang-jwt/jwt"
	"github.com/mpraski/api-gateway/crypto"
)

type (
	JWT struct {
		token  *jwt.Token
		claims *Claims
	}

	JWTParser struct {
		publicKey *rsa.PublicKey
	}

	// Claims that are stored in the JWT.
	Claims struct {
		jwt.StandardClaims
		Roles []string `json:"roles"`
	}
)

func (j *JWT) String() string {
	return j.token.Raw
}

func (j *JWT) Token() *jwt.Token {
	return j.token
}

func (c *Claims) Parse() error {
	if err := c.StandardClaims.Valid(); err != nil {
		return fmt.Errorf("failed to validate standard claims: %w", err)
	}

	return nil
}

func NewJWTParser(publicKey io.Reader) (*JWTParser, error) {
	p, err := crypto.ParsePublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &JWTParser{publicKey: p}, nil
}

func (p *JWTParser) Parse(data string) (Token, error) {
	token, err := jwt.ParseWithClaims(data, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return p.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	if err := claims.Parse(); err != nil {
		return nil, fmt.Errorf("failed to to parse claims: %w", err)
	}

	return &JWT{token: token, claims: claims}, nil
}
