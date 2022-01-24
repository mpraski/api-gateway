package secret

import "context"

type (
	Secret = []byte

	Getter interface {
		Get(context.Context, string) (Secret, error)
	}
)
