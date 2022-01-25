package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/mpraski/api-gateway/authentication"
	"github.com/mpraski/api-gateway/proxy"
	"github.com/mpraski/api-gateway/service"
	"github.com/mpraski/api-gateway/store"
	"github.com/mpraski/api-gateway/token"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type (
	timeouts struct {
		ReadTimeout     time.Duration `default:"5s"`
		WriteTimeout    time.Duration `default:"10s"`
		IdleTimeout     time.Duration `default:"15s"`
		ShutdownTimeout time.Duration `default:"30s"`
	}

	input struct {
		Proxy struct {
			Config string `required:"true"`
		}
		Server struct {
			timeouts
			Address string `default:":8080"`
		}
		Internal struct {
			timeouts
			Address string `default:":8081"`
		}
		Observability struct {
			timeouts
			Address string `default:":9090"`
		}
	}
)

var (
	// Health check
	healthy int32
	app     = "api_gateway"
	// Metrics
	requestsRoutedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_gateway_requests_routed_total",
		Help: "The total number of routed requests",
	}, []string{"method", "path", "code"})
	requestsRoutedDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "api_gateway_requests_routed_duration_seconds",
		Help:    "The histogram of routed request duration in seconds",
		Buckets: prometheus.DefBuckets,
	})
)

const depsSize = 2

func main() {
	rootCtx := context.Background()

	rand.Seed(time.Now().UnixNano())

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)
	logger.Println("server is starting...")

	var i input
	if err := envconfig.Process(app, &i); err != nil {
		logger.Fatalf("failed to load input: %v\n", err)
	}

	var (
		proxyConfig = strings.NewReader(i.Proxy.Config)
		storeGetter = store.NewMemoryStore()
	)

	factory, err := authentication.NewFactory(proxyConfig, storeGetter)
	if err != nil {
		logger.Fatalf("failed to initialize authentication scheme factory: %v\n", err)
	}

	scheme, err := factory.New(rootCtx, authentication.Phantom)
	if err != nil {
		logger.Fatalf("failed to initialize phantom authentication scheme: %v\n", err)
	}

	var (
		deps sync.WaitGroup
		done = make(chan bool)
		quit = make(chan os.Signal, 1)
	)

	deps.Add(depsSize)

	internal := newInternalServer(&i)

	go func() {
		logger.Println("starting internal server at", i.Internal.Address)
		deps.Done()

		if errs := internal.ListenAndServe(); errs != nil && errs != http.ErrServerClosed {
			logger.Fatalf("failed to start internal server on %s: %v\n", i.Internal.Address, errs)
		}
	}()

	observability := newObservabilityServer(&i)

	go func() {
		logger.Println("starting observability server at", i.Observability.Address)
		deps.Done()

		if errs := observability.ListenAndServe(); errs != nil && errs != http.ErrServerClosed {
			logger.Fatalf("failed to start observability server on %s: %v\n", i.Observability.Address, errs)
		}
	}()

	_, _ = proxyConfig.Seek(0, io.SeekStart)

	p, err := proxy.New(proxyConfig, scheme)
	if err != nil {
		logger.Fatalf("failed to initialize proxy: %v\n", err)
	}

	h := p.Handler()
	h = proxy.WithMetrics(requestsRoutedTotal, requestsRoutedDuration)(h)
	h = proxy.WithLogging(logger)(h)

	main := &http.Server{
		Addr:         i.Server.Address,
		ReadTimeout:  i.Server.ReadTimeout,
		WriteTimeout: i.Server.WriteTimeout,
		IdleTimeout:  i.Server.IdleTimeout,
		Handler:      h,
	}

	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		logger.Println("server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), i.Server.ShutdownTimeout)
		defer cancel()

		main.SetKeepAlivesEnabled(false)
		internal.SetKeepAlivesEnabled(false)
		observability.SetKeepAlivesEnabled(false)

		if err := main.Shutdown(ctx); err != nil {
			logger.Fatalf("failed to gracefully shutdown the server: %v\n", err)
		}

		if err := internal.Shutdown(ctx); err != nil {
			logger.Fatalf("failed to gracefully shutdown internal server: %v\n", err)
		}

		if err := observability.Shutdown(ctx); err != nil {
			logger.Fatalf("failed to gracefully shutdown observability server: %v\n", err)
		}

		close(done)
	}()

	deps.Wait()

	logger.Println("server is ready to handle requests at", i.Server.Address)
	atomic.StoreInt32(&healthy, 1)

	if err := main.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("failed to listen on %s: %v\n", i.Server.Address, err)
	}

	<-done
	logger.Println("server stopped")
}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func newInternalServer(cfg *input) *http.Server {
	router := http.NewServeMux()

	router.Handle("/internal/tokens", http.HandlerFunc(service.NewTokenReferenceServer(nil).HandleAssociation))

	return &http.Server{
		Addr:         cfg.Internal.Address,
		ReadTimeout:  cfg.Internal.ReadTimeout,
		WriteTimeout: cfg.Internal.WriteTimeout,
		IdleTimeout:  cfg.Internal.IdleTimeout,
		Handler:      router,
	}
}

func newObservabilityServer(cfg *input) *http.Server {
	router := http.NewServeMux()
	router.Handle("/healthz", healthz())
	router.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:         cfg.Observability.Address,
		ReadTimeout:  cfg.Observability.ReadTimeout,
		WriteTimeout: cfg.Observability.WriteTimeout,
		IdleTimeout:  cfg.Observability.IdleTimeout,
		Handler:      router,
	}
}

// nolint:deadcode,unused // Only for local testing
func test() {
	key, err := os.Open("examples/key.pem")
	if err != nil {
		panic(err)
	}
	defer key.Close()

	pkey, err := os.Open("examples/pkey.pem")
	if err != nil {
		panic(err)
	}
	defer key.Close()

	ri, err := token.NewReferenceIssuer(pkey)
	if err != nil {
		panic(err)
	}

	rp, err := token.NewReferenceParser(key)
	if err != nil {
		panic(err)
	}

	t, err := ri.Issue()
	if err != nil {
		panic(err)
	}

	if _, err = rp.Parse(t.String()); err != nil {
		panic(err)
	}
}
