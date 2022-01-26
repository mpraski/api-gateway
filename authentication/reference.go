package authentication

import (
	"context"
	"fmt"
	"time"

	"github.com/mpraski/api-gateway/store"
	"github.com/mpraski/api-gateway/token"
)

type (
	TokenReference struct {
		setter          store.Setter
		valueParser     token.Parser
		referenceParser token.Parser
		referenceIssuer token.Issuer
	}
)

func NewTokenReference(
	setter store.Setter,
	valueParser token.Parser,
	referenceParser token.Parser,
	referenceIssuer token.Issuer,
) *TokenReference {
	return &TokenReference{
		setter:          setter,
		valueParser:     valueParser,
		referenceParser: referenceParser,
		referenceIssuer: referenceIssuer,
	}
}

func (t *TokenReference) Make(ctx context.Context, value string, expiration time.Duration) (token.Token, error) {
	v, err := t.valueParser.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse value token: %w", err)
	}

	r, err := t.referenceIssuer.Issue()
	if err != nil {
		return nil, fmt.Errorf("failed to issue reference token: %w", err)
	}

	if err := t.setter.Set(ctx, r.String(), v.String(), expiration); err != nil {
		return nil, fmt.Errorf("failed to associate tokens: %w", err)
	}

	return r, nil
}

func (t *TokenReference) Delete(ctx context.Context, reference string) error {
	r, err := t.referenceParser.Parse(reference)
	if err != nil {
		return fmt.Errorf("failed to parse reference token: %w", err)
	}

	if err := t.setter.Del(ctx, r.String()); err != nil {
		return fmt.Errorf("failed to delete token association: %w", err)
	}

	return nil
}
