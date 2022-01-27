package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type (
	Audience []string

	Introspection struct {
		Active    bool                   `json:"active"`
		Extra     map[string]interface{} `json:"ext"`
		Subject   string                 `json:"sub,omitempty"`
		Username  string                 `json:"username"`
		Audience  Audience               `json:"aud,omitempty"`
		TokenType string                 `json:"token_type"`
		Issuer    string                 `json:"iss"`
		ClientID  string                 `json:"client_id,omitempty"`
		Scope     string                 `json:"scope,omitempty"`
		Expires   int64                  `json:"exp"`
		TokenUse  string                 `json:"token_use"`
	}

	OAuth2InstrospectionAuthenticator struct {
		client  *http.Client
		baseURL string
	}
)

const (
	tokenLength   = 2
	clientTimeout = time.Second * 15
)

var (
	ErrTokenMissing    = errors.New("failed to extract token from header")
	ErrInvalidAudience = errors.New("invalid audience value")
	ErrNotAccessToken  = errors.New("token in use is not an access token")
	ErrTokenInactive   = errors.New("token is inactive")
	ErrTokenExpired    = errors.New("token is expired")
)

func (a *Audience) UnmarshalJSON(b []byte) error {
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*a = Audience{single}

		return nil
	}

	var multiple []string
	if err := json.Unmarshal(b, &multiple); err == nil {
		*a = multiple

		return nil
	}

	return ErrInvalidAudience
}

func (i *Introspection) Validate() error {
	if len(i.TokenUse) > 0 && i.TokenUse != "access_token" {
		return ErrNotAccessToken
	}

	if !i.Active {
		return ErrTokenInactive
	}

	if i.Expires > 0 && time.Unix(i.Expires, 0).Before(time.Now()) {
		return ErrTokenExpired
	}

	if len(i.Extra) == 0 {
		i.Extra = map[string]interface{}{}
	}

	i.Extra["username"] = i.Username
	i.Extra["client_id"] = i.ClientID
	i.Extra["scope"] = i.Scope

	if len(i.Audience) != 0 {
		i.Extra["aud"] = i.Audience
	}

	return nil
}

func NewOAuth2InstrospectionAuthenticator(baseURL string) *OAuth2InstrospectionAuthenticator {
	return &OAuth2InstrospectionAuthenticator{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: clientTimeout,
		},
	}
}

func (a *OAuth2InstrospectionAuthenticator) Authenticate(r *http.Request) error {
	t, ok := extractToken(r)
	if !ok {
		return ErrTokenMissing
	}

	i, err := a.introspect(r.Context(), t)
	if err != nil {
		return fmt.Errorf("failed to introspect token: %w", err)
	}

	if err := i.Validate(); err != nil {
		return fmt.Errorf("failed to validate introspection: %w", err)
	}

	r.Header.Set("X-Subject", i.Subject)
	r.Header.Set("X-Issuer", i.Issuer)
	r.Header.Set("X-Client-ID", i.ClientID)
	r.Header.Set("X-Scope", i.Scope)
	r.Header.Del("X-Audience")

	for _, a := range i.Audience {
		r.Header.Add("X-Audience", a)
	}

	return nil
}

func (a *OAuth2InstrospectionAuthenticator) introspect(ctx context.Context, token string) (*Introspection, error) {
	body := url.Values{"token": {token}}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspection request: %w", err)
	}

	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	p, err := a.client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to make introspection request: %w", err)
	}

	defer p.Body.Close()

	var i Introspection
	if err := json.NewDecoder(p.Body).Decode(&i); err != nil {
		return nil, fmt.Errorf("failed to decode introspection response: %w", err)
	}

	return &i, nil
}

func extractToken(r *http.Request) (value string, found bool) {
	if arr := strings.Split(r.Header.Get("Authorization"), " "); len(arr) == tokenLength {
		found = true
		value = arr[1]
	}

	return
}
