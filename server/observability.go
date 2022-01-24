package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewObservability(config Config, healthz http.Handler) *http.Server {
	router := http.NewServeMux()
	router.Handle("/healthz", healthz)
	router.Handle("/metrics", promhttp.Handler())

	return newServer(config, router)
}
