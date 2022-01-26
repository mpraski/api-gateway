package secret

import "context"

type (
	Secret = []byte

	Source interface {
		Get(context.Context, string) (Secret, error)
	}
)
