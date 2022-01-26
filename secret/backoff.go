package secret

import (
	"context"
	"time"
)

type BackoffSource struct {
	tries   int
	backoff time.Duration
	source  Source
}

func NewBackoffSource(tries int, backoff time.Duration, source Source) *BackoffSource {
	return &BackoffSource{tries: tries, backoff: backoff, source: source}
}

func (s *BackoffSource) Get(ctx context.Context, name string) (Secret, error) {
	var (
		secret []byte
		err    error
	)

	for i := 0; i < s.tries; i++ {
		time.Sleep(s.backoff)

		if secret, err = s.source.Get(ctx, name); err == nil {
			return secret, nil
		}
	}

	return nil, err
}
