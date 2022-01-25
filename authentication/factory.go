package authentication

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mpraski/api-gateway/secret"
	"github.com/mpraski/api-gateway/store"
	"github.com/mpraski/api-gateway/token"
	"golang.org/x/sync/errgroup"
)

type Factory struct {
	store   store.Getter
	secrets *secretsConfig
}

var (
	ErrUknownScheme       = errors.New("unknown authentication scheme")
	ErrUknownSecretSource = errors.New("unknown secret source")
)

func NewFactory(configDataSource io.Reader, getter store.Getter) (*Factory, error) {
	secrets, err := parseSecrets(configDataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &Factory{secrets: secrets, store: getter}, nil
}

func (f *Factory) New(ctx context.Context, scheme SchemeType) (Scheme, error) {
	if scheme == Phantom {
		r, v, err := loadPublicKeys(ctx, f.secrets)
		if err != nil {
			return nil, fmt.Errorf("failed to load public keys: %w", err)
		}

		rp, err := token.NewReferenceParser(bytes.NewReader(r))
		if err != nil {
			return nil, fmt.Errorf("failed to create reference token parser: %w", err)
		}

		vp, err := token.NewJWTParser(bytes.NewReader(v))
		if err != nil {
			return nil, fmt.Errorf("failed to create value token parser: %w", err)
		}

		return NewPhantomAuthenticator(f.store, rp, vp), nil
	}

	return nil, ErrUknownScheme
}

func loadPublicKeys(ctx context.Context, secrets *secretsConfig) (reference, value []byte, err error) {
	getter, closer, err := makeGetter(secrets.Source)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create secret getter: %w", err)
	}

	defer closer()

	var (
		parser = func(name string, key *[]byte) func() error {
			return func() error {
				r, err := getter.Get(ctx, name)
				if err != nil {
					return err
				}

				*key = r

				return nil
			}
		}
	)

	group, ctx := errgroup.WithContext(ctx)

	group.Go(parser(secrets.Reference.PublicKey, &reference))
	group.Go(parser(secrets.Value.PublicKey, &value))

	if err := group.Wait(); err != nil {
		return nil, nil, fmt.Errorf("failed to load public key: %w", err)
	}

	return reference, value, nil
}

const backoff = 3

func makeGetter(source string) (secret.Getter, func(), error) {
	switch source {
	case "gsm":
		gsm, err := secret.NewGoogleSecretManager()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create google secret manager client: %w", err)
		}

		return secret.NewBackoffStore(backoff, backoff*time.Second, gsm), gsm.Close, nil

	case "file":
		return secret.NewFileStore(), func() {}, nil
	}

	return nil, nil, ErrUknownSecretSource
}
