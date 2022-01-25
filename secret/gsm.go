package secret

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type GoogleSecretManager struct {
	client *secretmanager.Client
}

func NewGoogleSecretManager() (*GoogleSecretManager, error) {
	c, err := secretmanager.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize google secret manager client: %w", err)
	}

	return &GoogleSecretManager{client: c}, nil
}

func (m *GoogleSecretManager) Get(ctx context.Context, name string) (Secret, error) {
	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{Name: name}

	r, err := m.client.AccessSecretVersion(ctx, accessRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret: %w", err)
	}

	return r.Payload.Data, nil
}

func (m *GoogleSecretManager) Close() { _ = m.client.Close() }
