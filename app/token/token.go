package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type (
	Client struct {
		baseURL string
		client  *http.Client
	}

	request struct {
		AccessToken string `json:"access_token"`
	}

	response struct {
		IdentityToken string `json:"identity_token"`
	}
)

var ErrInvalidSession = errors.New("session is invalid")

func NewClient(baseURL string, client *http.Client) *Client {
	return &Client{baseURL: baseURL, client: client}
}

func (c *Client) GetIdentity(ctx context.Context, accessToken string) (string, error) {
	var (
		b bytes.Buffer
		a = request{AccessToken: accessToken}
	)

	if err := json.NewEncoder(&b).Encode(a); err != nil {
		return "", fmt.Errorf("failed to encode request to json: %w", err)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, &b)
	if err != nil {
		return "", fmt.Errorf("failed to create new request: %w", err)
	}

	s, err := c.client.Do(r)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}

	defer s.Body.Close()

	if s.StatusCode != http.StatusOK {
		return "", ErrInvalidSession
	}

	var i response
	if err := json.NewDecoder(s.Body).Decode(&i); err != nil {
		return "", fmt.Errorf("failed to decode json to session: %w", err)
	}

	return i.IdentityToken, nil
}
