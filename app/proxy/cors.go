package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type cors struct {
	enabled          bool
	onlyPreflight    bool
	allowCredentials bool
	allowedOrigins   []string
	allowedHeaders   []string
	allowedMethods   []string
	exposedHeaders   []string
}

var recognizedMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

func (c *cors) handlePreflight(w http.ResponseWriter, r *http.Request) bool {
	var (
		h = w.Header()
		o = r.Header.Get("Origin")
	)

	if r.Method != http.MethodOptions {
		return false
	}

	h.Add("Vary", "Origin")
	h.Add("Vary", "Access-Control-Request-Method")
	h.Add("Vary", "Access-Control-Request-Headers")

	if o == "" {
		return false
	}

	if !c.isOriginAllowed(o) {
		return false
	}

	reqMethod := r.Header.Get("Access-Control-Request-Method")
	if !c.isMethodAllowed(reqMethod) {
		return false
	}

	reqHeaders := parseHeaderList(r.Header.Get("Access-Control-Request-Headers"))
	if !c.areHeadersAllowed(reqHeaders) {
		return false
	}

	if c.areAllOriginsAllowed() {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Access-Control-Allow-Origin", o)
	}

	h.Set("Access-Control-Allow-Methods", strings.ToUpper(reqMethod))

	if len(reqHeaders) > 0 {
		h.Set("Access-Control-Allow-Headers", strings.Join(reqHeaders, ", "))
	}

	if c.allowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}

	return true
}

func (c *cors) handleActualRequest(w http.ResponseWriter, r *http.Request) {
	var (
		h = w.Header()
		o = r.Header.Get("Origin")
	)

	h.Add("Vary", "Origin")

	if o == "" {
		return
	}

	if !c.isOriginAllowed(o) {
		return
	}

	if !c.isMethodAllowed(r.Method) {
		return
	}

	if c.areAllOriginsAllowed() {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Access-Control-Allow-Origin", o)
	}

	if len(c.exposedHeaders) > 0 {
		h.Set("Access-Control-Expose-Headers", strings.Join(c.exposedHeaders, ", "))
	}

	if c.allowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
}

func (c *cors) isOriginAllowed(o string) bool {
	if c.areAllOriginsAllowed() {
		return true
	}

	o = strings.ToLower(o)

	for i := range c.allowedOrigins {
		if o == c.allowedOrigins[i] {
			return true
		}
	}

	return false
}

func (c *cors) areAllOriginsAllowed() bool {
	return len(c.allowedOrigins) == 1 && c.allowedOrigins[0] == "*"
}

func (c *cors) isMethodAllowed(m string) bool {
	if len(c.allowedMethods) == 0 {
		return false
	}

	m = strings.ToUpper(m)

	if m == http.MethodOptions {
		return true
	}

	for i := range c.allowedMethods {
		if m == c.allowedMethods[i] {
			return true
		}
	}

	return false
}

func (c *cors) areHeadersAllowed(hs []string) bool {
	if len(hs) == 0 {
		return true
	}

	for i := range hs {
		var f bool

		for _, h := range c.allowedHeaders {
			if f = h == http.CanonicalHeaderKey(hs[i]); f {
				break
			}
		}

		if !f {
			return false
		}
	}

	return true
}

func (c *cors) parse(r *configRoute) error {
	if r.Cors == nil {
		return nil
	}

	if r.Cors != nil {
		if r.Cors.Enabled != nil {
			c.enabled = *r.Cors.Enabled
		}

		if r.Cors.OnlyPreflight != nil {
			c.onlyPreflight = *r.Cors.OnlyPreflight
		}

		if c.onlyPreflight {
			c.enabled = false
		}

		if r.Cors.AllowCredentials != nil {
			c.allowCredentials = *r.Cors.AllowCredentials
		}

		if r.Cors.AllowedOrigins != nil {
			c.allowedOrigins = *r.Cors.AllowedOrigins

			for i := range c.allowedOrigins {
				c.allowedOrigins[i] = strings.TrimSpace(c.allowedOrigins[i])

				if c.allowedOrigins[i] == "*" {
					continue
				}

				if _, err := url.Parse(c.allowedOrigins[i]); err != nil {
					return fmt.Errorf("origin %q is not valid", c.allowedOrigins[i])
				}
			}
		}

		if r.Cors.AllowedHeaders != nil {
			c.allowedHeaders = *r.Cors.AllowedHeaders

			for i := range c.allowedHeaders {
				c.allowedHeaders[i] = http.CanonicalHeaderKey(strings.TrimSpace(c.allowedHeaders[i]))
			}
		}

		if r.Cors.ExposedHeaders != nil {
			c.exposedHeaders = *r.Cors.ExposedHeaders

			for i := range c.exposedHeaders {
				c.exposedHeaders[i] = http.CanonicalHeaderKey(strings.TrimSpace(c.exposedHeaders[i]))
			}
		}

		if r.Cors.AllowedMethods != nil {
			c.allowedMethods = *r.Cors.AllowedMethods

			for i := range c.allowedMethods {
				c.allowedMethods[i] = strings.ToUpper(strings.TrimSpace(c.allowedMethods[i]))

				if !isMethodRecognized(c.allowedMethods[i]) {
					return fmt.Errorf("method %q is not valid", c.allowedMethods[i])
				}
			}
		}
	}

	return nil
}

func (c *cors) validate() error {
	if !c.enabled {
		return nil
	}

	if len(c.allowedHeaders) == 0 {
		return ErrNoAllowedHeaders
	}

	if len(c.allowedMethods) == 0 {
		return ErrNoAllowedMethods
	}

	if len(c.allowedOrigins) == 0 {
		return ErrNoAllowedOrigins
	}

	return nil
}

func isMethodRecognized(m string) bool {
	for i := range recognizedMethods {
		if m == recognizedMethods[i] {
			return true
		}
	}

	return false
}
