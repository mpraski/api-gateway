package store

import (
	"context"
	"time"
)

type Setter interface {
	Set(context.Context, string, string, time.Duration) error
}
