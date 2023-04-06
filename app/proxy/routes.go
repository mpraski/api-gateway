package proxy

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/dghubble/trie"
	"gopkg.in/yaml.v2"
)

type (
	routes struct{ t *trie.PathTrie }

	route struct {
		cors      cors
		target    *url.URL
		rateLimit rateLimit
		authz     authorization
		prefix    string
		rewrite   string
	}

	match struct {
		path  string
		route *route
	}

	configRoute struct {
		Prefix        string               `yaml:"prefix"`
		Target        *string              `yaml:"target"`
		Rewrite       *string              `yaml:"rewrite"`
		Authorization *configAuthorization `yaml:"authorization"`
		RateLimit     *configRateLimit     `yaml:"rateLimit"`
		Cors          *configCors          `yaml:"cors"`
		Routes        []configRoute        `yaml:"routes,flow"`
	}

	configAuthorization struct {
		Via    *string `yaml:"via"`
		From   *string `yaml:"from"`
		Policy *string `yaml:"policy"`
	}

	configCors struct {
		Enabled          *bool     `yaml:"enabled"`
		OnlyPreflight    *bool     `yaml:"onlyPreflight"`
		AllowCredentials *bool     `yaml:"allowCredentials"`
		AllowedOrigins   *[]string `yaml:"allowedOrigins,flow"`
		AllowedHeaders   *[]string `yaml:"allowedHeaders,flow"`
		AllowedMethods   *[]string `yaml:"allowedMethods,flow"`
		ExposedHeaders   *[]string `yaml:"exposedHeaders,flow"`
	}

	configRateLimit struct {
		Enabled  *bool          `yaml:"enabled"`
		Limit    *uint64        `yaml:"limit"`
		Duration *time.Duration `yaml:"duration"`
	}
)

var (
	ErrInvalidRateLimit         = errors.New("invalid rate limit")
	ErrInvalidRateLimitDuration = errors.New("invalid rate limit duration")
	ErrNoAllowedHeaders         = errors.New("no headers allowed in CORS")
	ErrNoAllowedOrigins         = errors.New("no origins allowed in CORS")
	ErrNoAllowedMethods         = errors.New("no methods allowed in CORS")
	ErrNilPolicy                = errors.New("authorization policy cannot be nil")
	ErrNilFrom                  = errors.New("authorization from cannot be nil when policy is permitted or enforced")
	ErrNilVia                   = errors.New("authorization via cannot be nil when policy is permitted or enforced")
)

func parseRoutes(configData string) (*routes, error) {
	var c struct {
		Routes []configRoute `yaml:"routes,flow"`
	}

	if err := yaml.NewDecoder(strings.NewReader(configData)).Decode(&c); err != nil {
		return nil, fmt.Errorf("failed to decode config data: %w", err)
	}

	pathTrie := trie.NewPathTrie()

	if err := addRoutes(pathTrie, "/", nil, c.Routes); err != nil {
		return nil, fmt.Errorf("failed to add routes: %w", err)
	}

	return &routes{t: pathTrie}, nil
}

func addRoutes(t *trie.PathTrie, p string, a *route, r []configRoute) error {
	if r == nil {
		return nil
	}

	for i := range r {
		if r[i].Prefix == "" {
			continue
		}

		m := path.Join(p, r[i].Prefix)

		var (
			u *url.URL
			e error
		)

		if r[i].Target != nil {
			u, e = url.Parse(*r[i].Target)
			if e != nil {
				return fmt.Errorf("failed to parse target: %w", e)
			}
		}

		var re string
		if r[i].Rewrite != nil {
			re = *r[i].Rewrite
		}

		authz, err := parseAuthorization(&r[i])
		if err != nil {
			return fmt.Errorf("failed to parse authorization: %w", err)
		}

		var l rateLimit
		if a != nil {
			l = a.rateLimit
		}

		l.parse(&r[i])

		var o cors
		if a != nil {
			o = a.cors
		}

		if err := o.parse(&r[i]); err != nil {
			return fmt.Errorf("failed to parse cors: %w", err)
		}

		c := route{
			cors:      o,
			target:    u,
			rewrite:   re,
			rateLimit: l,
			prefix:    r[i].Prefix,
			authz:     authz,
		}

		if a != nil {
			if c.target == nil && a.target != nil {
				c.target = a.target
			}

			if c.rewrite == "" && a.rewrite != "" {
				c.rewrite = a.rewrite
			}

			if c.authz.via == nullVia && a.authz.via != nullVia {
				c.authz.via = a.authz.via
			}

			if c.authz.from == nullFrom && a.authz.from != nullFrom {
				c.authz.from = a.authz.from
			}

			if c.authz.policy == nullPolicy && a.authz.policy != nullPolicy {
				c.authz.policy = a.authz.policy
			}
		}

		if err := c.validate(); err != nil {
			return fmt.Errorf("route %q to %q is invalid: %w", c.prefix, c.target, err)
		}

		if !t.Put(m, &c) {
			return fmt.Errorf("route %q to %q is already mapped", c.prefix, c.target)
		}

		if err := addRoutes(t, m, &c, r[i].Routes); err != nil {
			return err
		}
	}

	return nil
}

func (r *route) validate() error {
	var err error

	if err = r.authz.validate(); err != nil {
		return fmt.Errorf("authz configuration invalid: %w", err)
	}

	if err = r.cors.validate(); err != nil {
		return fmt.Errorf("cors configuration invalid: %w", err)
	}

	if err = r.rateLimit.validate(); err != nil {
		return fmt.Errorf("rate limiter configuration invalid: %w", err)
	}

	return nil
}

func (r *routes) match(p string) (match, bool) {
	var (
		l int
		t *route
		e = r.t.WalkPath(p, func(key string, value interface{}) error {
			//nolint:errcheck //always known
			t = value.(*route)
			l = len(key)

			return nil
		})
	)

	if e != nil || t == nil || t.target == nil {
		return match{}, false
	}

	m := match{path: p, route: t}

	if m.route.rewrite != "" {
		m.path = singleJoiningSlash(m.route.rewrite, p[l:])
	}

	return m, true
}

func singleJoiningSlash(a, b string) string {
	var (
		aSlash = strings.HasSuffix(a, "/")
		bSlash = strings.HasPrefix(b, "/")
	)

	switch {
	case b == "":
		return a
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	}

	return a + b
}
