package cache

import (
	"fmt"
	"time"

	"github.com/dgraph-io/ristretto"
)

type (
	Cache interface {
		Get(string) ([]byte, bool)
		Set(string, []byte, time.Duration)
	}

	InMemory struct {
		cache *ristretto.Cache
	}
)

const bufferItems = 64

func NewInMemory(numCounters, maxCost int64) (*InMemory, error) {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: numCounters,
		MaxCost:     maxCost,
		BufferItems: bufferItems,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	return &InMemory{cache: c}, nil
}

func (c *InMemory) Get(key string) ([]byte, bool) {
	i, f := c.cache.Get(key)
	if !f {
		return nil, false
	}

	v, ok := i.([]byte)
	if !ok {
		return nil, false
	}

	return v, true
}

func (c *InMemory) Set(key string, value []byte, expiry time.Duration) {
	_ = c.cache.SetWithTTL(key, value, 1, expiry)
}
