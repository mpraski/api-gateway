package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/mpraski/api-gateway/app/authentication"
	"github.com/mpraski/api-gateway/app/proxy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type input struct {
	Config string `required:"true"`
	Server struct {
		Address         string        `default:":8080"`
		ReadTimeout     time.Duration `split_words:"true" default:"5s"`
		WriteTimeout    time.Duration `split_words:"true" default:"10s"`
		IdleTimeout     time.Duration `split_words:"true" default:"15s"`
		ShutdownTimeout time.Duration `split_words:"true" default:"30s"`
	}
	Observability struct {
		Address string `default:":9090"`
	}
}

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

func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.WarnLevel)
}

func main() {
	var i input
	if err := envconfig.Process(app, &i); err != nil {
		log.Fatalf("failed to load input: %v\n", err)
	}

	proxyConfig := strings.NewReader(i.Config)

	schemes, err := authentication.MakeSchemes(proxyConfig)
	if err != nil {
		log.Fatalf("failed to initialize authentication schemes: %v\n", err)
	}

	var (
		done = make(chan bool)
		quit = make(chan os.Signal, 1)
	)

	observability := newObservabilityServer(&i)

	go func() {
		log.Println("starting observability server at", i.Observability.Address)

		if errs := observability.ListenAndServe(); errs != nil && errs != http.ErrServerClosed {
			log.Fatalf("failed to start observability server on %s: %v\n", i.Observability.Address, errs)
		}
	}()

	_, _ = proxyConfig.Seek(0, io.SeekStart)

	p, err := proxy.New(proxyConfig, schemes)
	if err != nil {
		log.Fatalf("failed to initialize proxy: %v\n", err)
	}

	h := p.Handler()
	h = proxy.WithMetrics(requestsRoutedTotal, requestsRoutedDuration)(h)
	h = proxy.WithLogging()(h)

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
		log.Println("server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), i.Server.ShutdownTimeout)
		defer cancel()

		main.SetKeepAlivesEnabled(false)
		observability.SetKeepAlivesEnabled(false)

		if err := main.Shutdown(ctx); err != nil {
			log.Fatalf("failed to gracefully shutdown the server: %v\n", err)
		}

		if err := observability.Shutdown(ctx); err != nil {
			log.Fatalf("failed to gracefully shutdown observability server: %v\n", err)
		}

		close(done)
	}()

	log.Println("server is ready to handle requests at", i.Server.Address)
	atomic.StoreInt32(&healthy, 1)

	if err := main.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to listen on %s: %v\n", i.Server.Address, err)
	}

	<-done
	log.Println("server stopped")
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

func newObservabilityServer(cfg *input) *http.Server {
	router := http.NewServeMux()
	router.Handle("/healthz", healthz())
	router.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:         cfg.Observability.Address,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		Handler:      router,
	}
}
