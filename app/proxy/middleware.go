package proxy

import (
	"log"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

// LoggingWriter persists the response status code.
type loggingWriter struct {
	http.ResponseWriter
	Code int
}

const decimalBase = 10

func WithMetrics(
	counter *prometheus.CounterVec,
	histogram prometheus.Histogram,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w = newLoggingWriter(w)
			timer := prometheus.NewTimer(histogram)
			defer func() {
				timer.ObserveDuration()
				counter.WithLabelValues(
					r.Method,
					r.URL.Path,
					strconv.FormatInt(int64(w.(*loggingWriter).Code), decimalBase),
				).Inc()
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func WithLogging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w = newLoggingWriter(w)
			defer func() {
				logger.Println(
					r.Method,
					r.URL.Path,
					w.(*loggingWriter).Code,
					r.RemoteAddr,
					r.UserAgent(),
				)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func newLoggingWriter(w http.ResponseWriter) *loggingWriter {
	if w, ok := w.(*loggingWriter); ok {
		return w
	}

	return &loggingWriter{w, http.StatusOK}
}

func (w *loggingWriter) WriteHeader(code int) {
	w.Code = code
	w.ResponseWriter.WriteHeader(code)
}
