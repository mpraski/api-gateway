package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"cloud.google.com/go/logging"
	"github.com/go-redis/redis/v8"
	"github.com/hellofresh/health-go/v4"
	"github.com/kelseyhightower/envconfig"
	"github.com/mpraski/api-gateway/app/proxy"
	"github.com/mpraski/api-gateway/app/ratelimit"
	"github.com/mpraski/api-gateway/app/secret"
	"github.com/mpraski/api-gateway/app/token"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type config struct {
	Debug  bool
	Delay  time.Duration `default:"1s"`
	Config string        `required:"true"`
	Server struct {
		Address struct {
			Public        string `default:":8080"`
			Observability string `default:":9090"`
		}
		ReadTimeout       time.Duration `split_words:"true" default:"30s"`
		WriteTimeout      time.Duration `split_words:"true" default:"30s"`
		IdleTimeout       time.Duration `split_words:"true" default:"120s"`
		ReadyTimeout      time.Duration `split_words:"true" default:"5s"`
		ShutdownTimeout   time.Duration `split_words:"true" default:"10s"`
		ReadHeaderTimeout time.Duration `split_words:"true" default:"5s"`
	}
	Identity struct {
		BaseURL string        `required:"true" split_words:"true"`
		Timeout time.Duration `default:"15s"`
	}
	Redis struct {
		Address  string
		Database int `default:"0"`
	}
	Secrets struct {
		RedisCertificate string `split_words:"true"`
	}
	Project struct {
		ID string `required:"true"`
	}
}

var (
	// Health check
	ready int32
	app   = "api_gateway"
	// Errors
	errShutdown           = errors.New("shutdown in progress")
	errTooManyGoroutines  = errors.New("too many goroutines")
	errRedisMisconfigured = errors.New("redis is misconfigured")
	errCertificateInvalid = errors.New("failed to decode PEM certificate")
)

func main() {
	ctx := context.Background()

	var cfg config
	if err := envconfig.Process(app, &cfg); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	time.Sleep(cfg.Delay)

	var opts []option.ClientOption
	if cfg.Debug {
		opts = append(opts,
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
	}

	c, err := logging.NewClient(ctx, cfg.Project.ID, opts...)
	if err != nil {
		log.Fatalf("failed to setup logging client: %v", err)
	}

	defer func() {
		if err := c.Close(); err != nil {
			log.Fatalf("failed to close logging client: %v", err)
		}
	}()

	l := c.Logger(app, logging.RedirectAsJSON(os.Stdout))

	if err := run(ctx, &cfg, l); err != nil {
		l.StandardLogger(logging.Critical).Fatalf("failed to run app: %v", err)
	}
}

func run(ctx context.Context, cfg *config, lg *logging.Logger) error {
	var (
		appLog = lg.StandardLogger(logging.Info)
		errLog = lg.StandardLogger(logging.Critical)
		client = token.NewClient(cfg.Identity.BaseURL, &http.Client{Timeout: cfg.Identity.Timeout})
	)

	rateLimiter, closer, err := newRateLimiter(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize rate limiter: %w", err)
	}

	defer func() {
		if err = closer(); err != nil {
			errLog.Fatalf("failed to close rate limiter: %v", err)
		}
	}()

	if rateLimiter == nil {
		appLog.Println("not using rate limiting")
	} else {
		appLog.Println("using rate limiting")
	}

	p, err := proxy.New(cfg.Config, client, lg, rateLimiter)
	if err != nil {
		return fmt.Errorf("failed to initialize proxy: %w", err)
	}

	checks, err := newHealthChecks()
	if err != nil {
		return fmt.Errorf("failed to setup health checks: %w", err)
	}

	var (
		warm sync.WaitGroup
		done = make(chan struct{})
		quit = make(chan os.Signal, 1)

		publicServer = &http.Server{
			Addr:              cfg.Server.Address.Public,
			ReadTimeout:       cfg.Server.ReadTimeout,
			WriteTimeout:      cfg.Server.WriteTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
			Handler:           p.Handler(),
			BaseContext: func(net.Listener) context.Context {
				return ctx
			},
		}
		observabilityServer = newServer(ctx, cfg, cfg.Server.Address.Observability, func(m *http.ServeMux) {
			m.Handle("/livez", checks[0])
			m.Handle("/readyz", checks[1])
		})
		runServer = func(server *http.Server) {
			warm.Done()
			appLog.Println("starting server at", server.Addr)

			if errs := server.ListenAndServe(); errs != nil && errs != http.ErrServerClosed {
				errLog.Fatalf("failed to start server at %s: %v", server.Addr, errs)
			}
		}
	)

	warm.Add(2)

	go runServer(publicServer)
	go runServer(observabilityServer)

	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit

		appLog.Println("app is shutting down...")
		atomic.StoreInt32(&ready, 0)

		publicServer.SetKeepAlivesEnabled(false)
		observabilityServer.SetKeepAlivesEnabled(false)

		time.Sleep(cfg.Server.ReadyTimeout)

		c, cancel := context.WithTimeout(ctx, cfg.Server.ShutdownTimeout)
		defer cancel()

		if err := publicServer.Shutdown(c); err != nil {
			errLog.Fatalf("failed to gracefully shutdown public server: %v", err)
		}

		if err := observabilityServer.Shutdown(c); err != nil {
			errLog.Fatalf("failed to gracefully shutdown observability server: %v", err)
		}

		close(done)
	}()

	warm.Wait()

	atomic.StoreInt32(&ready, 1)

	appLog.Println("app started")

	<-done

	appLog.Println("app stopped")

	return nil
}

func newServer(ctx context.Context, cfg *config, address string, f func(*http.ServeMux)) *http.Server {
	r := http.NewServeMux()

	f(r)

	return &http.Server{
		Addr:              address,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		Handler:           r,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
}

const maxGoroutines = 1000

func newHealthChecks() ([2]http.Handler, error) {
	l, err := health.New(health.WithChecks(
		health.Config{
			Name:    "goroutine",
			Timeout: time.Second * 5,
			Check: func(_ context.Context) error {
				if runtime.NumGoroutine() > maxGoroutines {
					return errTooManyGoroutines
				}

				return nil
			},
		},
	))

	if err != nil {
		return [2]http.Handler{}, fmt.Errorf("failed to set up health checks: %w", err)
	}

	r, err := health.New(health.WithChecks(
		health.Config{
			Name:    "shutdown",
			Timeout: time.Second,
			Check: func(_ context.Context) error {
				if atomic.LoadInt32(&ready) == 0 {
					return errShutdown
				}

				return nil
			},
		},
	))

	if err != nil {
		return [2]http.Handler{}, fmt.Errorf("failed to set up health checks: %w", err)
	}

	return [2]http.Handler{l.Handler(), r.Handler()}, nil
}

var emptyCloseFunc = func() error { return nil }

func newRateLimiter(ctx context.Context, cfg *config) (ratelimit.HandleFunc, func() error, error) {
	if cfg.Debug {
		return nil, emptyCloseFunc, nil
	}

	if cfg.Redis.Address == "" || cfg.Secrets.RedisCertificate == "" {
		return nil, emptyCloseFunc, errRedisMisconfigured
	}

	gsm, gerr := secret.NewGoogleSecretManager(ctx, cfg.Project.ID)
	if gerr != nil {
		return nil, emptyCloseFunc, fmt.Errorf("failed to connect to GSM: %w", gerr)
	}

	defer gsm.Close()

	redisCert, rerr := gsm.Get(ctx, cfg.Secrets.RedisCertificate)
	if rerr != nil {
		return nil, emptyCloseFunc, fmt.Errorf("failed to fetch redis certificate: %w", rerr)
	}

	b, _ := pem.Decode(redisCert)
	if b == nil {
		return nil, emptyCloseFunc, errCertificateInvalid
	}

	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return nil, emptyCloseFunc, fmt.Errorf("failed to parse PEM certificate: %w", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(c)

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Address,
		DB:   cfg.Redis.Database,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		},
	})

	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		return nil, emptyCloseFunc, fmt.Errorf("failed to ping redis: %w", err)
	}

	var (
		rateLimiter = ratelimit.NewHandler(
			ratelimit.NewSortedSetStrategy(redisClient),
			ratelimit.KeyFromHeader("X-Forwarded-For"),
		)
		closeFunc = func() error {
			if err := redisClient.Close(); err != nil {
				return fmt.Errorf("failed to close redis client: %w", err)
			}

			return nil
		}
	)

	return rateLimiter, closeFunc, nil
}
