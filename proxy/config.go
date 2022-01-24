package proxy

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

type (
	service struct {
		Target        string   `yaml:"target"`
		TargetURL     *url.URL `yaml:"-"`
		Routes        []route  `yaml:"routes,flow"`
		Authenticated bool     `yaml:"authenticated"`
	}

	route struct {
		Prefix        string   `yaml:"prefix"`
		PrefixSlash   string   `yaml:"-"`
		Target        string   `yaml:"target"`
		TargetURL     *url.URL `yaml:"-"`
		Rewrite       string   `yaml:"rewrite"`
		Private       bool     `yaml:"private"`
		Authenticated bool     `yaml:"authenticated"`
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

	var routes = make([]route, 0, len(c.Services))

	for _, s := range c.Services {
		s := s

		u, err := url.Parse(s.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to parse service target host (%s): %w", s.Target, err)
		}

		for _, r := range s.Routes {
			r := r

			var (
				target      string
				targetURL   *url.URL
				prefix      = filepath.Clean(r.Prefix)
				prefixSlash = prefix
			)

			if !strings.HasSuffix(prefixSlash, "/") {
				prefixSlash += "/"
			}

			if r.Target != "" {
				ur, err := url.Parse(r.Target)
				if err != nil {
					return nil, fmt.Errorf("failed to parse route target host (%s): %w", r.Target, err)
				}

				target = r.Target
				targetURL = ur
			} else {
				target = s.Target
				targetURL = u
			}

			routes = append(routes, route{
				Prefix:        prefix,
				PrefixSlash:   prefixSlash,
				Target:        target,
				TargetURL:     targetURL,
				Rewrite:       r.Rewrite,
				Private:       r.Private,
				Authenticated: r.Authenticated,
			})
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return len(routes[i].Prefix) > len(routes[j].Prefix)
	})

	return routes, nil
}
