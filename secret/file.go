package secret

import (
	"context"
	"os"
)

type FileSource struct{}

func NewFileSource() *FileSource { return &FileSource{} }

func (s *FileSource) Get(_ context.Context, name string) (Secret, error) {
	return os.ReadFile(name)
}
