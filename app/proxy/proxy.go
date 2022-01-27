package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"

	"github.com/mpraski/api-gateway/app/authentication"
)

type (
	Proxy struct {
		routes  []route
		schemes authentication.Schemes
		proxy   *httputil.ReverseProxy
	}

	contextKey uint
)

const routeKey contextKey = 10

func New(configDataSource io.Reader, schemes authentication.Schemes) (*Proxy, error) {
	r, err := parseRoutes(configDataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy routes: %w", err)
	}

	return &Proxy{
		routes:  r,
		schemes: schemes,
		proxy:   newReverseProxy(),
	}, nil
}

func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var route *route

		for i := range p.routes {
			if p.routes[i].matches(r.URL.Path) {
				route = &p.routes[i]
				break
			}
		}

		if route == nil || route.Private {
			http.NotFound(w, r)
			return
		}

		if route.Authentication != nil {
			a, ok := p.schemes[*route.Authentication]
			if !ok {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			if err := a.Authenticate(r); err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
		} else {
			authentication.ClearHeaders(r)
		}

		r = r.WithContext(context.WithValue(r.Context(), routeKey, route))

		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))

		p.proxy.ServeHTTP(w, r)
	})
}

func newReverseProxy() *httputil.ReverseProxy {
	director := func(req *http.Request) {
		var (
			route        = req.Context().Value(routeKey).(*route)
			routePath    = req.URL.Path
			targetScheme = route.Target.Scheme
			targetHost   = route.Target.Host
			targetQuery  = route.Target.RawQuery
		)

		if targetScheme == "" {
			targetScheme = "http"
		}

		if route.Rewrite != "" {
			routePath = strings.TrimPrefix(routePath, route.Prefix)
			routePath = path.Join(route.Rewrite, routePath)
			req.URL.Path = routePath
		}

		req.URL.Host = targetHost
		req.URL.Scheme = targetScheme

		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}

		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "")
		}
	}

	return &httputil.ReverseProxy{Director: director}
}
