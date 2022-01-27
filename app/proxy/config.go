package proxy

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mpraski/api-gateway/app/authentication"
	"gopkg.in/yaml.v2"
)

type (
	authScheme *map[string]map[string]interface{}

	authConfig struct {
		Name string
		Args map[string]interface{}
	}

	service struct {
		Target         string        `yaml:"target"`
		Routes         []routeConfig `yaml:"routes,flow"`
		Authentication authScheme    `yaml:"authentication"`
	}

	routeConfig struct {
		Prefix         string     `yaml:"prefix"`
		Rewrite        string     `yaml:"rewrite"`
		Authentication authScheme `yaml:"authentication"`
	}

	route struct {
		Target         *url.URL
		Prefix         string
		PrefixSlash    string
		Rewrite        string
		Authentication *authConfig
	}
)

func (r *route) matches(urlPath string) bool {
	return r.Prefix == urlPath || strings.HasPrefix(urlPath, r.PrefixSlash)
}

func parseRoutes(configDataSource io.Reader) ([]route, error) {
	var c struct {
		Services map[string]service `yaml:"services,flow"`
	}

	if err := yaml.NewDecoder(configDataSource).Decode(&c); err != nil {
		return nil, fmt.Errorf("failed to decode config data: %w", err)
	}

	var routes = make([]route, 0)

	for _, s := range c.Services {
		s := s

		u, err := url.Parse(s.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to parse service target host (%s): %w", s.Target, err)
		}

		for _, r := range s.Routes {
			r := r

			var (
				prefix      = filepath.Clean(r.Prefix)
				prefixSlash = prefix
				auth        = s.Authentication
			)

			if !strings.HasSuffix(prefixSlash, "/") {
				prefixSlash += "/"
			}

			if r.Authentication != nil {
				auth = r.Authentication
			}

			var authCfg *authConfig

			if auth != nil {
				for _, a := range authentication.SupportedSchemes {
					if args, ok := (*auth)[a]; ok {
						authCfg = &authConfig{
							Name: a,
							Args: args,
						}
					}
				}
			}

			routes = append(routes, route{
				Prefix:         prefix,
				PrefixSlash:    prefixSlash,
				Target:         u,
				Rewrite:        r.Rewrite,
				Authentication: authCfg,
			})
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return len(routes[i].Prefix) > len(routes[j].Prefix)
	})

	return routes, nil
}
