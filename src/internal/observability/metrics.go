// TEACHING NOTES:
// Observability code keeps runtime counters in process so handlers and
// middleware can report operational behavior without exporting sensitive data.
// Useful Go concepts to notice:
// 1. Atomic counters avoid locks for simple high-frequency observations.
// 2. Snapshots copy current counter values into plain structs for logging/tests.
// 3. Low-cardinality labels keep logs useful without leaking user-controlled data.
package observability

import (
	"sync/atomic"
	"time"
)

const (
	RateLimitLoginIP      = "login_ip"
	RateLimitLoginAccount = "login_account"
	RateLimitSensitive    = "sensitive"
)

var requestDurationBuckets = []struct {
	label string
	limit time.Duration
}{
	{label: "le_50ms", limit: 50 * time.Millisecond},
	{label: "le_100ms", limit: 100 * time.Millisecond},
	{label: "le_250ms", limit: 250 * time.Millisecond},
	{label: "le_500ms", limit: 500 * time.Millisecond},
	{label: "le_1s", limit: time.Second},
	{label: "gt_1s", limit: 0},
}

type Metrics struct {
	requestsTotal atomic.Uint64
	status1xx     atomic.Uint64
	status2xx     atomic.Uint64
	status3xx     atomic.Uint64
	status4xx     atomic.Uint64
	status5xx     atomic.Uint64
	duration      [6]atomic.Uint64

	loginSuccess            atomic.Uint64
	loginFailure            atomic.Uint64
	loginLockoutHits        atomic.Uint64
	loginLockoutTransitions atomic.Uint64

	rateLimitLoginIP      atomic.Uint64
	rateLimitLoginAccount atomic.Uint64
	rateLimitSensitive    atomic.Uint64
}

type Snapshot struct {
	RequestsTotal uint64
	Status1xx     uint64
	Status2xx     uint64
	Status3xx     uint64
	Status4xx     uint64
	Status5xx     uint64
	Duration      map[string]uint64

	LoginSuccess            uint64
	LoginFailure            uint64
	LoginLockoutHits        uint64
	LoginLockoutTransitions uint64

	RateLimitLoginIP      uint64
	RateLimitLoginAccount uint64
	RateLimitSensitive    uint64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) RecordRequest(status int, duration time.Duration) {
	if m == nil {
		return
	}
	m.requestsTotal.Add(1)
	switch {
	case status >= 100 && status < 200:
		m.status1xx.Add(1)
	case status >= 200 && status < 300:
		m.status2xx.Add(1)
	case status >= 300 && status < 400:
		m.status3xx.Add(1)
	case status >= 400 && status < 500:
		m.status4xx.Add(1)
	case status >= 500:
		m.status5xx.Add(1)
	}
	m.duration[durationBucket(duration)].Add(1)
}

func (m *Metrics) RecordLoginSuccess() {
	if m != nil {
		m.loginSuccess.Add(1)
	}
}

func (m *Metrics) RecordLoginFailure() {
	if m != nil {
		m.loginFailure.Add(1)
	}
}

func (m *Metrics) RecordLoginLockoutHit() {
	if m != nil {
		m.loginLockoutHits.Add(1)
	}
}

func (m *Metrics) RecordLoginLockoutTransition() {
	if m != nil {
		m.loginLockoutTransitions.Add(1)
	}
}

func (m *Metrics) RecordRateLimitDenied(kind string) {
	if m == nil {
		return
	}
	switch kind {
	case RateLimitLoginIP:
		m.rateLimitLoginIP.Add(1)
	case RateLimitLoginAccount:
		m.rateLimitLoginAccount.Add(1)
	case RateLimitSensitive:
		m.rateLimitSensitive.Add(1)
	}
}

func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{Duration: durationSnapshot(nil)}
	}
	return Snapshot{
		RequestsTotal:           m.requestsTotal.Load(),
		Status1xx:               m.status1xx.Load(),
		Status2xx:               m.status2xx.Load(),
		Status3xx:               m.status3xx.Load(),
		Status4xx:               m.status4xx.Load(),
		Status5xx:               m.status5xx.Load(),
		Duration:                durationSnapshot(&m.duration),
		LoginSuccess:            m.loginSuccess.Load(),
		LoginFailure:            m.loginFailure.Load(),
		LoginLockoutHits:        m.loginLockoutHits.Load(),
		LoginLockoutTransitions: m.loginLockoutTransitions.Load(),
		RateLimitLoginIP:        m.rateLimitLoginIP.Load(),
		RateLimitLoginAccount:   m.rateLimitLoginAccount.Load(),
		RateLimitSensitive:      m.rateLimitSensitive.Load(),
	}
}

func durationBucket(duration time.Duration) int {
	for i, bucket := range requestDurationBuckets {
		if bucket.limit == 0 || duration <= bucket.limit {
			return i
		}
	}
	return len(requestDurationBuckets) - 1
}

func durationSnapshot(counters *[6]atomic.Uint64) map[string]uint64 {
	out := make(map[string]uint64, len(requestDurationBuckets))
	for i, bucket := range requestDurationBuckets {
		if counters == nil {
			out[bucket.label] = 0
			continue
		}
		out[bucket.label] = counters[i].Load()
	}
	return out
}
