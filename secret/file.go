package secret

import (
	"context"
	"os"
)

type FileStore struct{}

func NewFileStore() *FileStore { return &FileStore{} }

func (s *FileStore) Get(_ context.Context, name string) (Secret, error) {
	return os.ReadFile(name)
}
