package token

import (
	"errors"
	"fmt"
)

type (
	Token = fmt.Stringer

	Issuer interface {
		Issue() (Token, error)
	}

	Parser interface {
		Parse(string) (Token, error)
	}
)

var ErrTokenInvalid = errors.New("token is invalid")
