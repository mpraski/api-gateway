package store

import (
	"context"
	"time"
)

type Setter interface {
	Del(context.Context, string) error
	Set(context.Context, string, string, time.Duration) error
}
