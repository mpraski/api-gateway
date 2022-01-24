package store

import "context"

type Getter interface {
	Get(context.Context, string) (string, error)
}
