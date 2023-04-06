package ratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type (
	KeyFunc func(*http.Request) (string, error)

	HandleFunc func(http.ResponseWriter, *http.Request, Config) bool

	Middleware func(http.Handler) http.Handler

	Config struct {
		Limit    uint64
		Duration time.Duration
	}
)

const (
	rateLimitingState         = "Rate-Limiting-State"
	rateLimitingExpiresAt     = "Rate-Limiting-Expires-At"
	rateLimitingTotalRequests = "Rate-Limiting-Total-Requests"
)

func NewMiddleware(strategy Strategy, keyFunc KeyFunc, cfg Config) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k, err := keyFunc(r)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}

			l, err := strategy.Run(r.Context(), Request{Key: k, Limit: cfg.Limit, Duration: cfg.Duration})
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			e := w.Header()

			e.Set(rateLimitingState, stateStr[l.State])
			e.Set(rateLimitingExpiresAt, l.ExpiresAt.Format(time.RFC3339))
			e.Set(rateLimitingTotalRequests, strconv.FormatUint(l.TotalRequests, 10))

			if l.State == Deny {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func NewHandler(strategy Strategy, keyFunc KeyFunc) HandleFunc {
	return func(w http.ResponseWriter, r *http.Request, cfg Config) bool {
		k, err := keyFunc(r)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return false
		}

		l, err := strategy.Run(r.Context(), Request{Key: k, Limit: cfg.Limit, Duration: cfg.Duration})
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return false
		}

		e := w.Header()

		e.Set(rateLimitingState, stateStr[l.State])
		e.Set(rateLimitingExpiresAt, l.ExpiresAt.Format(time.RFC3339))
		e.Set(rateLimitingTotalRequests, strconv.FormatUint(l.TotalRequests, 10))

		if l.State == Deny {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return false
		}

		return true
	}
}

func KeyFromHeader(headers ...string) KeyFunc {
	return func(r *http.Request) (string, error) {
		var sb strings.Builder

		for _, k := range headers {
			sb.WriteString(strings.TrimSpace(r.Header.Get(k)))
			sb.WriteRune('-')
		}

		return sb.String(), nil
	}
}
