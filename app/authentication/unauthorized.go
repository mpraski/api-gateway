package authentication

import (
	"errors"
	"net/http"
)

type UnauthorizedAuthenticator struct{}

var ErrUnauthorized = errors.New("unauthorized")

func NewUnauthorizedAuthenticator() *UnauthorizedAuthenticator {
	return &UnauthorizedAuthenticator{}
}

func (a *UnauthorizedAuthenticator) Authenticate(r *http.Request, _ Args) error {
	ClearHeaders(r)

	return ErrUnauthorized
}
