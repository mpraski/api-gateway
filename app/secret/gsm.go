package secret

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

type GoogleSecretManager struct {
	projectID string
	client    *secretmanager.Client
}

var _ Source = (*GoogleSecretManager)(nil)

func NewGoogleSecretManager(ctx context.Context, projectID string) (*GoogleSecretManager, error) {
	c, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize google secret manager client: %w", err)
	}

	return &GoogleSecretManager{client: c, projectID: projectID}, nil
}

func (m *GoogleSecretManager) Get(ctx context.Context, name string) (Secret, error) {
	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", m.projectID, name),
	}

	r, err := m.client.AccessSecretVersion(ctx, accessRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret: %w", err)
	}

	return r.Payload.Data, nil
}

func (m *GoogleSecretManager) Close() { _ = m.client.Close() }
