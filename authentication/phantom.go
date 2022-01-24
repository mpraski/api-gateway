package authentication

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mpraski/api-gateway/store"
	"github.com/mpraski/api-gateway/token"
)

type PhantomAuthenticator struct {
	getter          store.Getter
	referenceParser token.Parser
	valueParser     token.Parser
}

const tokenLength = 2

var ErrTokenMissing = errors.New("failed to extract phantom token from header")

func NewPhantomAuthenticator(
	getter store.Getter,
	referenceParser token.Parser,
	valueParser token.Parser,
) *PhantomAuthenticator {
	return &PhantomAuthenticator{
		getter:          getter,
		referenceParser: referenceParser,
		valueParser:     valueParser,
	}
}

func (a *PhantomAuthenticator) Authenticate(r *http.Request) error {
	t, ok := extractToken(r)
	if !ok {
		return ErrTokenMissing
	}

	p, err := a.referenceParser.Parse(t)
	if err != nil {
		return fmt.Errorf("failed to parse reference token: %w", err)
	}

	h, err := a.getter.Get(r.Context(), p.String())
	if err != nil {
		return fmt.Errorf("failed to retrieve value token: %w", err)
	}

	j, err := a.valueParser.Parse(h)
	if err != nil {
		return fmt.Errorf("failed to parse value token: %w", err)
	}

	_ = j

	r.Header.Set(origAuthorizationHeader, "Bearer "+t)
	r.Header.Set(authorizationHeader, "Bearer "+h)

	return nil
}

func extractToken(r *http.Request) (value string, found bool) {
	if arr := strings.Split(r.Header.Get(authorizationHeader), " "); len(arr) == tokenLength {
		found = true
		value = arr[1]
	}

	return
}
