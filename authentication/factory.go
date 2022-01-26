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
	getter          store.Getter
	setter          store.Setter
	referenceParser token.Parser
	referenceIssuer token.Issuer
	valueParser     token.Parser
}

var (
	ErrUknownScheme       = errors.New("unknown authentication scheme")
	ErrUknownSecretSource = errors.New("unknown secret source")
)

func NewFactory(
	ctx context.Context,
	configDataSource io.Reader,
	getter store.Getter,
	setter store.Setter,
) (*Factory, error) {
	secrets, err := parseSecrets(configDataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	keys, err := loadKeys(ctx, secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to load keys: %w", err)
	}

	rp, err := token.NewReferenceParser(bytes.NewReader(keys[0]))
	if err != nil {
		return nil, fmt.Errorf("failed to create reference token parser: %w", err)
	}

	ri, err := token.NewReferenceIssuer(bytes.NewReader(keys[1]))
	if err != nil {
		return nil, fmt.Errorf("failed to create reference token issuer: %w", err)
	}

	vp, err := token.NewJWTParser(bytes.NewReader(keys[2]))
	if err != nil {
		return nil, fmt.Errorf("failed to create value token parser: %w", err)
	}

	return &Factory{
		getter:          getter,
		setter:          setter,
		referenceParser: rp,
		referenceIssuer: ri,
		valueParser:     vp,
	}, nil
}

func (f *Factory) New(scheme SchemeType) (Scheme, error) {
	if scheme == Phantom {
		return NewPhantomAuthenticator(f.getter, f.referenceParser, f.valueParser), nil
	}

	return nil, ErrUknownScheme
}

func (f *Factory) NewReference() *TokenReference {
	return NewTokenReference(
		f.setter,
		f.valueParser,
		f.referenceParser,
		f.referenceIssuer,
	)
}

func loadKeys(ctx context.Context, secrets *secretsConfig) ([3][]byte, error) {
	source, closer, err := makeSource(secrets.Source)
	if err != nil {
		return [3][]byte{}, fmt.Errorf("failed to create secret getter: %w", err)
	}

	defer closer()

	group, ctx := errgroup.WithContext(ctx)

	var (
		keys  [3][]byte
		parse = func(name string, key *[]byte) func() error {
			return func() error {
				r, err := source.Get(ctx, name)
				if err != nil {
					return fmt.Errorf("failed to load secret key: %w", err)
				}

				*key = r

				return nil
			}
		}
	)

	group.Go(parse(secrets.Reference.PublicKey, &keys[0]))
	group.Go(parse(secrets.Reference.PrivateKey, &keys[1]))
	group.Go(parse(secrets.Value.PublicKey, &keys[2]))

	if err := group.Wait(); err != nil {
		return [3][]byte{}, fmt.Errorf("failed to load key: %w", err)
	}

	return keys, nil
}

const backoff = 3

func makeSource(sourceName string) (secret.Source, func(), error) {
	switch sourceName {
	case "gsm":
		gsm, err := secret.NewGoogleSecretManager()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create google secret manager client: %w", err)
		}

		return secret.NewBackoffSource(backoff, backoff*time.Second, gsm), gsm.Close, nil

	case "file":
		return secret.NewFileSource(), func() {}, nil
	}

	return nil, nil, ErrUknownSecretSource
}
