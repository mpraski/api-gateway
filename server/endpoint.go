package server

import (
	"net/http"
	"time"
)

type (
	Config struct {
		Address         string
		ReadTimeout     time.Duration
		WriteTimeout    time.Duration
		IdleTimeout     time.Duration
		ShutdownTimeout time.Duration
	}
)

func newServer(config Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         config.Address,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
		Handler:      handler,
	}
}
