package authentication

import (
	"fmt"
	"io"
	"net/http"
)

type (
	Scheme interface {
		Authenticate(*http.Request) error
	}

	Schemes map[string]Scheme
)

const (
	Unauthorized        = "unauthorized"
	OAuth2Introspection = "oauth2-introspection"
)

var sensitiveHeaders = []string{
	"X-Subject",
	"X-Issuer",
	"X-Client-ID",
	"X-Scope",
	"X-Audience",
	"Authorization",
}

func MakeSchemes(configDataSource io.Reader) (Schemes, error) {
	c, err := parseConfig(configDataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	schemes := make(map[string]Scheme)

	schemes[Unauthorized] = NewUnauthorizedAuthenticator()

	if c.OAuth2Introspection != nil {
		s, err := NewOAuth2InstrospectionAuthenticator(c.OAuth2Introspection.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s scheme: %w", OAuth2Introspection, err)
		}

		schemes[OAuth2Introspection] = s
	}

	return schemes, nil
}

func ClearHeaders(r *http.Request) {
	for _, h := range sensitiveHeaders {
		r.Header.Del(h)
	}
}
