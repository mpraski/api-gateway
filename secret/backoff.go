package secret

import (
	"context"
	"time"
)

type BackoffStore struct {
	tries   int
	backoff time.Duration
	getter  Getter
}

func NewBackoffStore(tries int, backoff time.Duration, getter Getter) *BackoffStore {
	return &BackoffStore{tries: tries, backoff: backoff, getter: getter}
}

func (s *BackoffStore) Get(ctx context.Context, name string) (Secret, error) {
	var (
		secret []byte
		err    error
	)

	for i := 0; i < s.tries; i++ {
		time.Sleep(s.backoff)

		if secret, err = s.getter.Get(ctx, name); err != nil {
			return secret, nil
		}
	}

	return nil, err
}
