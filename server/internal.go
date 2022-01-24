package server

import (
	"net/http"
)

func NewInternal(config Config) *http.Server {
	router := http.NewServeMux()
	router.Handle("/internal", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))

	return newServer(config, router)
}
