package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

type (
	// Input represents the required program configuration.
	Input struct {
		Port      int    `default:"8080"`
		PublicKey string `required:"true" split_words:"true"`
		Config    string `required:"true"`
	}

	// Config represents the required proxy configuration.
	Config struct {
		Services map[string]Service `yaml:"services,flow"`
	}

	// Service represents a collection of routes.
	Service struct {
		Target        string   `yaml:"target"`
		TargetURL     *url.URL `yaml:"-"`
		Authenticated bool     `yaml:"authenticated"`
		Routes        []Route  `yaml:"routes,flow"`
	}

	// Route is the basic unit of routing.
	Route struct {
		Prefix        string   `yaml:"prefix"`
		PrefixSlash   string   `yaml:"-"`
		Target        string   `yaml:"target"`
		TargetURL     *url.URL `yaml:"-"`
		Rewrite       string   `yaml:"rewrite"`
		Private       bool     `yaml:"private"`
		Authenticated bool     `yaml:"authenticated"`
	}

	// Claims that are stored in the JWT.
	Claims struct {
		jwt.StandardClaims
		AccountID uuid.UUID `json:"-"`
		Roles     []string  `json:"roles"`
	}

	// Keystore holds the public key required for JWT verification.
	Keystore struct {
		PublicKeyBytes []byte
		PublicKey      *ecdsa.PublicKey
	}

	// LoggingWriter persists the response status code.
	LoggingWriter struct {
		http.ResponseWriter
		Code int
	}

	// ContextKey for progating context values.
	ContextKey uint
)

const (
	RouteKey ContextKey = 10
	// Headers
	AudienceHeader      = "X-Audience"
	AccountIDHeader     = "X-Account-ID"
	AccountRolesHeader  = "X-Account-Roles"
	AuthorizationHeader = "Authorization"
	// Timeouts
	ReadTimeout     = 5 * time.Second
	WriteTimeout    = 10 * time.Second
	IdleTimeout     = 15 * time.Second
	ShutdownTimeout = 30 * time.Second
)

var (
	// Misc
	healthy         int32
	app             = "api_gateway"
	senstiveHeaders = []string{
		AudienceHeader,
		AccountIDHeader,
		AccountRolesHeader,
		AuthorizationHeader,
	}
	// Errors
	ErrTokenMissing = errors.New("token is missing from the header")
	ErrTokenInvalid = errors.New("token is invalid")
	// Metrics
	RequestsRouted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_gateway_requests_routed_total",
		Help: "The total number of routed requests",
	}, []string{"method", "path", "code"})
)

func main() {
	logger := log.New(os.Stdout, "http: ", log.LstdFlags)
	logger.Println("Server is starting...")

	var input Input
	if err := envconfig.Process(app, &input); err != nil {
		logger.Fatalf("Failed to load input: %v\n", err)
	}

	routes, err := ParseConfig(input.Config)
	if err != nil {
		logger.Fatalf("Failed to load config: %v\n", err)
	}

	keystore, err := NewKeystore(input.PublicKey)
	if err != nil {
		logger.Fatalf("Failed to initialize keystore: %v\n", err)
	}

	var (
		done       = make(chan bool)
		quit       = make(chan os.Signal, 1)
		listenAddr = fmt.Sprintf(":%d", input.Port)
	)

	router := http.NewServeMux()
	router.Handle("/healthz", Healthz())
	router.Handle("/metrics", promhttp.Handler())
	router.Handle("/", WithLogging(logger)(
		WithMetrics(RequestsRouted)(
			NewHandler(routes, keystore, NewReverseProxy()),
		),
	))

	server := &http.Server{
		Addr:         listenAddr,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
		Handler:      router,
	}

	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		logger.Println("Server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)

		if err := server.Shutdown(ctx); err != nil {
			logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}

		close(done)
	}()

	logger.Println("Server is ready to handle requests at", listenAddr)
	atomic.StoreInt32(&healthy, 1)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Could not listen on %s: %v\n", listenAddr, err)
	}

	<-done
	logger.Println("Server stopped")
}

func ParseConfig(configData string) ([]Route, error) {
	var c Config
	if err := yaml.NewDecoder(strings.NewReader(configData)).Decode(&c); err != nil {
		return nil, fmt.Errorf("failed to decode config data: %w", err)
	}

	var routes = make([]Route, 0, len(c.Services))

	for _, s := range c.Services {
		s := s

		u, err := url.Parse(s.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to parse service target host (%s): %w", s.Target, err)
		}

		for _, r := range s.Routes {
			r := r

			var (
				target       string
				targetURL    *url.URL
				prefix       = filepath.Clean(r.Prefix)
				prefixSlash  = prefix
				autneticated = s.Authenticated
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

			if r.Authenticated {
				autneticated = r.Authenticated
			}

			routes = append(routes, Route{
				Prefix:        prefix,
				PrefixSlash:   prefixSlash,
				Target:        target,
				TargetURL:     targetURL,
				Rewrite:       r.Rewrite,
				Private:       r.Private,
				Authenticated: autneticated,
			})
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return len(routes[i].Prefix) > len(routes[j].Prefix)
	})

	return routes, nil
}

func (c *Claims) Parse() error {
	if err := c.StandardClaims.Valid(); err != nil {
		return fmt.Errorf("failed to validate standard claims: %w", err)
	}

	id, err := uuid.Parse(c.StandardClaims.Subject)
	if err != nil {
		return fmt.Errorf("failed to to parse account ID: %w", err)
	}

	c.AccountID = id

	return nil
}

func (r *Route) Matches(urlPath string) bool {
	return r.Prefix == urlPath || strings.HasPrefix(urlPath, r.PrefixSlash)
}

func NewKeystore(publicKey string) (*Keystore, error) {
	blockPub, _ := pem.Decode([]byte(publicKey))
	if blockPub == nil {
		return nil, errors.New("public certificate is invalid")
	}

	genericPublicKey, err := x509.ParsePKIXPublicKey(blockPub.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &Keystore{
		PublicKey:      genericPublicKey.(*ecdsa.PublicKey),
		PublicKeyBytes: []byte(publicKey),
	}, nil
}

func NewHandler(routes []Route, keystore *Keystore, proxy *httputil.ReverseProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var route *Route

		for i := range routes {
			if routes[i].Matches(r.URL.Path) {
				route = &routes[i]
				break
			}
		}

		if route == nil || route.Private {
			http.NotFound(w, r)
			return
		}

		if route.Authenticated {
			token, err := ParseToken(r, keystore)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			claims, err := VerifyToken(token)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			for _, h := range senstiveHeaders {
				r.Header.Del(h)
			}

			r.Header.Set(AudienceHeader, claims.Audience)
			r.Header.Set(AccountIDHeader, claims.AccountID.String())

			for _, role := range claims.Roles {
				r.Header.Add(AccountRolesHeader, role)
			}
		} else {
			for _, h := range senstiveHeaders {
				r.Header.Del(h)
			}
		}

		r = r.WithContext(context.WithValue(r.Context(), RouteKey, route))

		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))

		proxy.ServeHTTP(w, r)
	})
}

const tokenLength = 2

func ExtractToken(r *http.Request) (found bool, token string) {
	if arr := strings.Split(r.Header.Get(AuthorizationHeader), " "); len(arr) == tokenLength {
		found = true
		token = arr[1]
	}

	return
}

func ParseToken(r *http.Request, k *Keystore) (*jwt.Token, error) {
	ok, tokenStr := ExtractToken(r)
	if !ok {
		return nil, ErrTokenMissing
	}

	return jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return k.PublicKey, nil
	})
}

func VerifyToken(token *jwt.Token) (*Claims, error) {
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		if err := claims.Parse(); err != nil {
			return nil, fmt.Errorf("failed to to parse claims: %w", err)
		}

		return claims, nil
	}

	return nil, ErrTokenInvalid
}

func NewReverseProxy() *httputil.ReverseProxy {
	director := func(req *http.Request) {
		var (
			route        = req.Context().Value(RouteKey).(*Route)
			routePath    = req.URL.Path
			targetScheme = route.TargetURL.Scheme
			targetHost   = route.TargetURL.Host
			targetQuery  = route.TargetURL.RawQuery
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

func NewLoggingWriter(w http.ResponseWriter) *LoggingWriter {
	if w, ok := w.(*LoggingWriter); ok {
		return w
	}

	return &LoggingWriter{w, http.StatusOK}
}

func (w *LoggingWriter) WriteHeader(code int) {
	w.Code = code
	w.ResponseWriter.WriteHeader(code)
}

func Healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

const decimalBase = 10

func WithMetrics(counter *prometheus.CounterVec) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w = NewLoggingWriter(w)
			defer func() {
				counter.WithLabelValues(
					r.Method,
					r.URL.Path,
					strconv.FormatInt(int64(w.(*LoggingWriter).Code), decimalBase),
				).Inc()
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func WithLogging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w = NewLoggingWriter(w)
			defer func() {
				logger.Println(
					r.Method,
					r.URL.Path,
					w.(*LoggingWriter).Code,
					r.RemoteAddr,
					r.UserAgent(),
				)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
