package httpapi

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"sync"
	"time"
)

type rateLimitRule struct {
	group  string
	burst  float64
	window time.Duration
}

var rateLimitRules = map[string]rateLimitRule{
	"POST /v1/console/signup":             {group: "password-auth", burst: 10, window: time.Minute},
	"POST /v1/console/login":              {group: "password-auth", burst: 10, window: time.Minute},
	"POST /v1/signup":                     {group: "password-auth", burst: 10, window: time.Minute},
	"POST /v1/login":                      {group: "password-auth", burst: 10, window: time.Minute},
	"POST /v1/oauth/google/nonce":         {group: "password-auth", burst: 10, window: time.Minute},
	"POST /v1/oauth/google":               {group: "password-auth", burst: 10, window: time.Minute},
	"POST /v1/authorize":                  {group: "hosted-auth", burst: 10, window: time.Minute},
	"POST /v1/token":                      {group: "hosted-token", burst: 20, window: time.Minute},
	"POST /v1/password-reset/request":     {group: "email-actions", burst: 3, window: 15 * time.Minute},
	"POST /v1/email/verification/request": {group: "email-actions", burst: 3, window: 15 * time.Minute},
	"POST /v1/password-reset/confirm":     {group: "token-confirm", burst: 5, window: time.Minute},
	"POST /v1/email/verification/confirm": {group: "token-confirm", burst: 5, window: time.Minute},
}

const (
	maxRateLimitEntries = 8192
	rateLimitIdleTTL    = time.Hour
)

type rateLimitBucket struct {
	tokens    float64
	updatedAt time.Time
	lastSeen  time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]rateLimitBucket
	requests uint64
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]rateLimitBucket)}
}

func (l *rateLimiter) allow(key string, rule rateLimitRule, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok {
		l.makeRoom(now)
		bucket = rateLimitBucket{tokens: rule.burst, updatedAt: now, lastSeen: now}
	}
	if now.After(bucket.updatedAt) {
		refillRate := rule.burst / rule.window.Seconds()
		bucket.tokens = math.Min(rule.burst, bucket.tokens+now.Sub(bucket.updatedAt).Seconds()*refillRate)
		bucket.updatedAt = now
	}
	bucket.lastSeen = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		l.buckets[key] = bucket
		return true, 0
	}
	l.buckets[key] = bucket
	refillRate := rule.burst / rule.window.Seconds()
	retryAfter := time.Duration(math.Ceil((1-bucket.tokens)/refillRate)) * time.Second
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}

func (l *rateLimiter) makeRoom(now time.Time) {
	l.requests++
	if l.requests%256 == 0 || len(l.buckets) >= maxRateLimitEntries {
		cutoff := now.Add(-rateLimitIdleTTL)
		for key, bucket := range l.buckets {
			if bucket.lastSeen.Before(cutoff) {
				delete(l.buckets, key)
			}
		}
	}
	if len(l.buckets) < maxRateLimitEntries {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, bucket := range l.buckets {
		if oldestKey == "" || bucket.lastSeen.Before(oldest) {
			oldestKey, oldest = key, bucket.lastSeen
		}
	}
	if oldestKey != "" {
		delete(l.buckets, oldestKey)
	}
}

func rateLimitMiddleware(next http.Handler, limiter *rateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rule, ok := rateLimitRules[r.Method+" "+r.URL.Path]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		key := rule.group + ":" + remoteIP(r.RemoteAddr)
		allowed, retryAfter := limiter.allow(key, rule, time.Now())
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return "unknown"
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown"
}
