package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"kcnotes/internal/observability"
)

func TestRequestMetricsRecordsStatusAndRequestID(t *testing.T) {
	metrics := observability.NewMetrics()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})
	handler := RequestID(RequestMetrics(slog.New(slog.NewTextHandler(io.Discard, nil)), metrics)(next))

	req := httptest.NewRequest(http.MethodPost, "/admin/posts", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusCreated)
	}
	if res.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected request id header")
	}
	snapshot := metrics.Snapshot()
	if snapshot.RequestsTotal != 1 || snapshot.Status2xx != 1 {
		t.Fatalf("unexpected request metrics: %+v", snapshot)
	}
}

func TestRequestMetricsDefaultsImplicitStatusToOK(t *testing.T) {
	metrics := observability.NewMetrics()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	handler := RequestMetrics(nil, metrics)(next)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	snapshot := metrics.Snapshot()
	if snapshot.RequestsTotal != 1 || snapshot.Status2xx != 1 {
		t.Fatalf("unexpected request metrics: %+v", snapshot)
	}
}
