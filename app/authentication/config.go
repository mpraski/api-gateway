package authentication

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v2"
)

type (
	config struct {
		OAuth2Introspection *oauth2introspection `yaml:"oauth2-introspection"`
	}

	oauth2introspection struct {
		BaseURL string `yaml:"baseUrl"`
	}
)

func parseConfig(configDataSource io.Reader) (*config, error) {
	var c struct {
		Authentication config `yaml:"authentication"`
	}

	if err := yaml.NewDecoder(configDataSource).Decode(&c); err != nil {
		return nil, fmt.Errorf("failed to decode config data: %w", err)
	}

	return &c.Authentication, nil
}
