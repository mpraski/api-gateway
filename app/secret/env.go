package secret

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
)

type EnvSource struct{}

func NewEnvSource() *EnvSource { return &EnvSource{} }

var ErrSecretNotFound = errors.New("secret_not_found")

var _ Source = (*EnvSource)(nil)

func (s *EnvSource) Get(_ context.Context, name string) (Secret, error) {
	if v := os.Getenv(name); v != "" {
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return []byte(v), nil
		}

		return b, nil
	}

	return nil, ErrSecretNotFound
}
