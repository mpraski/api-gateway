package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mpraski/api-gateway/store"
	"github.com/mpraski/api-gateway/token"
)

type TokenReference struct {
	setter          store.Setter
	valueParser     token.Parser
	referenceIssuer token.Issuer
}

const referenceExpiration = time.Hour

func (t *TokenReference) Make(ctx context.Context, value string) (token.Token, error) {
	v, err := t.valueParser.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse value token: %w", err)
	}

	r, err := t.referenceIssuer.Issue()
	if err != nil {
		return nil, fmt.Errorf("failed to issue reference token: %w", err)
	}

	if err := t.setter.Set(ctx, r.String(), v.String(), referenceExpiration); err != nil {
		return nil, fmt.Errorf("failed to associate tokens: %w", err)
	}

	return r, nil
}
