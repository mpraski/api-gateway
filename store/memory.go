package store

import (
	"context"
	"errors"
	"time"
)

type MemoryStore struct {
	data map[string]string
}

var ErrNotFound = errors.New("not found")

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]string)}
}

func (r *MemoryStore) Get(_ context.Context, key string) (string, error) {
	if v, ok := r.data[key]; ok {
		return v, nil
	}

	return "", ErrNotFound
}

func (r *MemoryStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	r.data[key] = value
	return nil
}

func (r *MemoryStore) Del(_ context.Context, key string) error {
	delete(r.data, key)
	return nil
}
