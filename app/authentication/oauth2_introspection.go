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

	"github.com/mpraski/api-gateway/app/cache"
)

type (
	Scope []string

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
		Scope     Scope                  `json:"scope,omitempty"`
		Expires   int64                  `json:"exp"`
		TokenUse  string                 `json:"token_use"`
	}

	OAuth2InstrospectionAuthenticator struct {
		client  *http.Client
		tokens  cache.Cache
		baseURL string
	}
)

const (
	tokenLength   = 2
	numCounters   = 10000
	maxCost       = 100000000
	expiry        = time.Minute
	clientTimeout = 30 * time.Second
)

var (
	ErrNotAccessToken       = errors.New("token in use is not an access token")
	ErrTokenMissing         = errors.New("failed to extract token from header")
	ErrTokenInactive        = errors.New("token is inactive")
	ErrTokenExpired         = errors.New("token is expired")
	ErrInsufficientScope    = errors.New("scope is insufficient")
	ErrInsufficientAudience = errors.New("audience is insufficient")
	ErrInvalidAudience      = errors.New("invalid audience value")
	ErrInvalidScope         = errors.New("invalid scope value")
)

func (s *Scope) UnmarshalJSON(b []byte) error {
	var scopes string
	if err := json.Unmarshal(b, &scopes); err == nil {
		*s = strings.Fields(scopes)

		return nil
	}

	return ErrInvalidScope
}

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

func (i *Introspection) Validate(args Args) error {
	if len(i.TokenUse) > 0 && i.TokenUse != "access_token" {
		return ErrNotAccessToken
	}

	if !i.Active {
		return ErrTokenInactive
	}

	if i.Expires > 0 && time.Unix(i.Expires, 0).Before(time.Now()) {
		return ErrTokenExpired
	}

	if requiredScopeVal, ok := args["requiredScope"]; ok {
		if requiredScope, ok := requiredScopeVal.([]string); ok {
			if !isContained(requiredScope, i.Scope) {
				return ErrInsufficientScope
			}
		}
	}

	if requiredAudienceVal, ok := args["requiredAudience"]; ok {
		if requiredAudience, ok := requiredAudienceVal.([]string); ok {
			if !isContained(requiredAudience, i.Audience) {
				return ErrInsufficientAudience
			}
		}
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

func NewOAuth2InstrospectionAuthenticator(baseURL string) (*OAuth2InstrospectionAuthenticator, error) {
	tokens, err := cache.NewInMemory(numCounters, maxCost)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize token cache: %w", err)
	}

	return &OAuth2InstrospectionAuthenticator{
		baseURL: baseURL,
		tokens:  tokens,
		client:  &http.Client{Timeout: clientTimeout},
	}, nil
}

func (a *OAuth2InstrospectionAuthenticator) Authenticate(r *http.Request, args Args) error {
	t, ok := extractToken(r)
	if !ok {
		return ErrTokenMissing
	}

	var (
		f   bool
		v   []byte
		i   *Introspection
		err error
	)

	if v, f = a.tokens.Get(t); f {
		if err = json.Unmarshal(v, i); err != nil {
			i = nil
		}
	}

	if i == nil {
		i, err = a.introspect(r.Context(), t)
		if err != nil {
			return fmt.Errorf("failed to introspect token: %w", err)
		}

		if err = i.Validate(args); err != nil {
			return fmt.Errorf("failed to validate introspection: %w", err)
		}

		if v, err = json.Marshal(i); err == nil {
			a.tokens.Set(t, v, expiry)
		}
	}

	ClearHeaders(r)

	r.Header.Set("X-Issuer", i.Issuer)
	r.Header.Set("X-Subject", i.Subject)
	r.Header.Set("X-Client-ID", i.ClientID)

	for _, s := range i.Scope {
		r.Header.Add("X-Scope", s)
	}

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

func isContained(a, b []string) bool {
	for _, i := range a {
		var f bool

		for _, j := range b {
			if f = i == j; f {
				break
			}
		}

		if !f {
			return false
		}
	}

	return true
}
