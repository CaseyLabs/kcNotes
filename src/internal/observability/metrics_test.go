package observability

import (
	"testing"
	"time"
)

func TestMetricsRecordsRequestStatusAndDuration(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordRequest(200, 40*time.Millisecond)
	metrics.RecordRequest(404, 700*time.Millisecond)
	metrics.RecordRequest(503, 2*time.Second)

	snapshot := metrics.Snapshot()
	if snapshot.RequestsTotal != 3 {
		t.Fatalf("requests total = %d, want 3", snapshot.RequestsTotal)
	}
	if snapshot.Status2xx != 1 || snapshot.Status4xx != 1 || snapshot.Status5xx != 1 {
		t.Fatalf("unexpected status counts: %+v", snapshot)
	}
	if snapshot.Duration["le_50ms"] != 1 {
		t.Fatalf("le_50ms bucket = %d, want 1", snapshot.Duration["le_50ms"])
	}
	if snapshot.Duration["le_1s"] != 1 {
		t.Fatalf("le_1s bucket = %d, want 1", snapshot.Duration["le_1s"])
	}
	if snapshot.Duration["gt_1s"] != 1 {
		t.Fatalf("gt_1s bucket = %d, want 1", snapshot.Duration["gt_1s"])
	}
}

func TestMetricsRecordsAuthAndRateLimitCounters(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordLoginSuccess()
	metrics.RecordLoginFailure()
	metrics.RecordLoginLockoutHit()
	metrics.RecordLoginLockoutTransition()
	metrics.RecordRateLimitDenied(RateLimitLoginIP)
	metrics.RecordRateLimitDenied(RateLimitLoginAccount)
	metrics.RecordRateLimitDenied(RateLimitSensitive)

	snapshot := metrics.Snapshot()
	if snapshot.LoginSuccess != 1 || snapshot.LoginFailure != 1 {
		t.Fatalf("unexpected login counts: %+v", snapshot)
	}
	if snapshot.LoginLockoutHits != 1 || snapshot.LoginLockoutTransitions != 1 {
		t.Fatalf("unexpected lockout counts: %+v", snapshot)
	}
	if snapshot.RateLimitLoginIP != 1 || snapshot.RateLimitLoginAccount != 1 || snapshot.RateLimitSensitive != 1 {
		t.Fatalf("unexpected rate limit counts: %+v", snapshot)
	}
}
