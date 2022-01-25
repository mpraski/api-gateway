package authentication

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v2"
)

type (
	secretsConfig struct {
		Source    string      `yaml:"source"`
		Reference tokenConfig `yaml:"reference"`
		Value     tokenConfig `yaml:"value"`
	}

	tokenConfig struct {
		PublicKey  string `yaml:"publicKey"`
		PrivateKey string `yaml:"privateKey"`
	}
)

func parseSecrets(configDataSource io.Reader) (*secretsConfig, error) {
	var s struct {
		Secrets secretsConfig `yaml:"secrets"`
	}

	if err := yaml.NewDecoder(configDataSource).Decode(&s); err != nil {
		return nil, fmt.Errorf("failed to decode config data: %w", err)
	}

	return &s.Secrets, nil
}
