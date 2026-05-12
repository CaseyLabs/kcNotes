package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"kcnotes/internal/observability"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// RequestMetrics records low-cardinality request totals and logs one structured
// completion line per request.
func RequestMetrics(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			elapsed := time.Since(start)
			if metrics != nil {
				metrics.RecordRequest(status, elapsed)
			}
			if logger != nil {
				logger.Info("http request completed",
					"method", r.Method,
					"route", requestRoutePattern(r),
					"status", status,
					"duration_ms", elapsed.Milliseconds(),
					"request_id", RequestIDFromContext(r),
				)
			}
		})
	}
}

func requestRoutePattern(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return "unmatched"
}
