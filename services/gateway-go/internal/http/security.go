package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	redis "github.com/redis/go-redis/v9"

	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/storage"
)

var (
	ErrAPIKeyRequired = errors.New("api_key_required")
	ErrAPIKeyInvalid  = errors.New("api_key_invalid")
)

// Authenticator validates caller credentials.
type Authenticator interface {
	Authenticate(apiKey string) error
}

type allowAllAuthenticator struct{}

func NewAllowAllAuthenticator() Authenticator {
	return allowAllAuthenticator{}
}

func (allowAllAuthenticator) Authenticate(_ string) error {
	return nil
}

// APIKeyAuthenticator validates requests against a static key set.
type APIKeyAuthenticator struct {
	allowed map[string]struct{}
}

func NewAPIKeyAuthenticator(keys []string) *APIKeyAuthenticator {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	return &APIKeyAuthenticator{allowed: allowed}
}

func (a *APIKeyAuthenticator) Authenticate(apiKey string) error {
	if len(a.allowed) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return ErrAPIKeyRequired
	}
	if _, ok := a.allowed[trimmed]; !ok {
		return ErrAPIKeyInvalid
	}
	return nil
}

// RateLimiter decides whether a request can proceed.
type RateLimiter interface {
	Allow(ctx context.Context, key string, now time.Time) (bool, error)
}

type noopRateLimiter struct{}

func NewNoopRateLimiter() RateLimiter {
	return noopRateLimiter{}
}

func (noopRateLimiter) Allow(_ context.Context, _ string, _ time.Time) (bool, error) {
	return true, nil
}

type memoryBucket struct {
	windowKey string
	count     int
}

// InMemoryRateLimiter is a simple fixed-window limiter for local fallback and tests.
type InMemoryRateLimiter struct {
	limit int
	mu    sync.Mutex
	items map[string]memoryBucket
}

func NewInMemoryRateLimiter(limit int) *InMemoryRateLimiter {
	return &InMemoryRateLimiter{
		limit: limit,
		items: make(map[string]memoryBucket),
	}
}

func (l *InMemoryRateLimiter) Allow(_ context.Context, key string, now time.Time) (bool, error) {
	if l == nil || l.limit <= 0 {
		return true, nil
	}

	windowKey := now.UTC().Format("200601021504")

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.items[key]
	if bucket.windowKey != windowKey {
		bucket = memoryBucket{windowKey: windowKey}
	}
	bucket.count++
	l.items[key] = bucket
	return bucket.count <= l.limit, nil
}

// RedisRateLimiter uses Redis INCR + EXPIRE for multi-instance request limiting.
type RedisRateLimiter struct {
	client redis.Cmdable
	prefix string
	limit  int
}

func NewRedisRateLimiter(client redis.Cmdable, prefix string, limit int) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		prefix: prefix,
		limit:  limit,
	}
}

func (l *RedisRateLimiter) Allow(ctx context.Context, key string, now time.Time) (bool, error) {
	if l == nil || l.client == nil || l.limit <= 0 {
		return true, nil
	}

	windowStart := now.UTC().Truncate(time.Minute)
	redisKey := fmt.Sprintf("%s:%s:%s", l.prefix, key, windowStart.Format("200601021504"))
	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		ttl := time.Until(windowStart.Add(time.Minute))
		if ttl <= 0 {
			ttl = time.Minute
		}
		if err := l.client.Expire(ctx, redisKey, ttl).Err(); err != nil {
			return false, err
		}
	}
	return int(count) <= l.limit, nil
}

// RuntimeMetrics holds non-persistent counters derived from gateway middleware behavior.
type RuntimeMetrics struct {
	authRejects      atomic.Int64
	rateLimitRejects atomic.Int64
}

func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{}
}

func (m *RuntimeMetrics) IncAuthRejects() {
	if m != nil {
		m.authRejects.Add(1)
	}
}

func (m *RuntimeMetrics) IncRateLimitRejects() {
	if m != nil {
		m.rateLimitRejects.Add(1)
	}
}

func (m *RuntimeMetrics) AuthRejects() int64 {
	if m == nil {
		return 0
	}
	return m.authRejects.Load()
}

func (m *RuntimeMetrics) RateLimitRejects() int64 {
	if m == nil {
		return 0
	}
	return m.rateLimitRejects.Load()
}

func (m *RuntimeMetrics) RenderPrometheus(metrics storage.OpsMetrics) string {
	lines := []string{
		renderGauge("trustops_riskops_total_ingests", metrics.TotalIngests),
		renderGauge("trustops_riskops_idempotent_replays_total", metrics.IdempotentReplays),
		renderGauge("trustops_riskops_pending_outbox_events", metrics.PendingOutboxEvents),
		renderGauge("trustops_riskops_dead_letter_outbox_rows", metrics.DeadLetterOutboxRows),
		renderGauge("trustops_riskops_audit_log_count", metrics.AuditLogCount),
		renderGauge("trustops_riskops_auth_rejects_total", m.AuthRejects()),
		renderGauge("trustops_riskops_rate_limit_rejects_total", m.RateLimitRejects()),
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderGauge(name string, value int64) string {
	return fmt.Sprintf("%s %d", name, value)
}
