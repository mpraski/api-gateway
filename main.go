package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/mpraski/api-gateway/proxy"
	"github.com/mpraski/api-gateway/token"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type input struct {
	Proxy struct {
		Port   int    `default:"8080"`
		Config string `required:"true"`
	}
	Server struct {
		ReadTimeout     time.Duration `default:"5s"`
		WriteTimeout    time.Duration `default:"10s"`
		IdleTimeout     time.Duration `default:"15s"`
		ShutdownTimeout time.Duration `default:"30s"`
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

func main() {
	rand.Seed(time.Now().UnixNano())

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)
	logger.Println("server is starting...")

	var i input
	if err := envconfig.Process(app, &i); err != nil {
		logger.Fatalf("failed to load input: %v\n", err)
	}

	var (
		done       = make(chan bool)
		quit       = make(chan os.Signal, 1)
		listenAddr = fmt.Sprintf(":%d", i.Proxy.Port)
	)

	p, err := proxy.New(strings.NewReader(i.Proxy.Config), nil)
	if err != nil {
		logger.Fatalf("failed to initialize proxy: %v\n", err)
	}

	h := p.Handler()
	h = proxy.WithMetrics(requestsRoutedTotal, requestsRoutedDuration)(h)
	h = proxy.WithLogging(logger)(h)

	router := http.NewServeMux()
	router.Handle("/healthz", healthz())
	router.Handle("/metrics", promhttp.Handler())
	router.Handle("/", h)

	server := &http.Server{
		Addr:         listenAddr,
		ReadTimeout:  i.Server.ReadTimeout,
		WriteTimeout: i.Server.WriteTimeout,
		IdleTimeout:  i.Server.IdleTimeout,
		Handler:      router,
	}

	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		logger.Println("server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), i.Server.ShutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)

		if err := server.Shutdown(ctx); err != nil {
			logger.Fatalf("failed to gracefully shutdown the server: %v\n", err)
		}

		close(done)
	}()

	logger.Println("server is ready to handle requests at", listenAddr)
	atomic.StoreInt32(&healthy, 1)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("failed to listen on %s: %v\n", listenAddr, err)
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
