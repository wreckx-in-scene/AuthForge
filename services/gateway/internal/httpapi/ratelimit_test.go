package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitMiddlewareByPeerAndGroup(t *testing.T) {
	var handled atomic.Int32
	handler := rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}), newRateLimiter())

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/login", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		req.Header.Set("X-Forwarded-For", "198.51.100.1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: got status %d, want %d", i+1, rec.Code, http.StatusNoContent)
		}
	}

	blocked := httptest.NewRequest(http.MethodPost, "/v1/login", nil)
	blocked.RemoteAddr = "203.0.113.10:12345"
	blocked.Header.Set("X-Forwarded-For", "198.51.100.2")
	blockedRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRec, blocked)
	if blockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("got status %d, want %d", blockedRec.Code, http.StatusTooManyRequests)
	}
	if blockedRec.Header().Get("Retry-After") == "" {
		t.Fatal("rate limited response missing Retry-After")
	}
	if handled.Load() != 10 {
		t.Fatalf("handler called %d times, want 10", handled.Load())
	}

	otherPeer := httptest.NewRequest(http.MethodPost, "/v1/login", nil)
	otherPeer.RemoteAddr = "203.0.113.11:12345"
	otherPeerRec := httptest.NewRecorder()
	handler.ServeHTTP(otherPeerRec, otherPeer)
	if otherPeerRec.Code != http.StatusNoContent {
		t.Fatalf("different peer got status %d, want %d", otherPeerRec.Code, http.StatusNoContent)
	}

	otherGroup := httptest.NewRequest(http.MethodPost, "/v1/password-reset/request", nil)
	otherGroup.RemoteAddr = "203.0.113.10:12345"
	otherGroupRec := httptest.NewRecorder()
	handler.ServeHTTP(otherGroupRec, otherGroup)
	if otherGroupRec.Code != http.StatusNoContent {
		t.Fatalf("different limit group got status %d, want %d", otherGroupRec.Code, http.StatusNoContent)
	}
}

func TestRateLimiterRefillsTokens(t *testing.T) {
	limiter := newRateLimiter()
	rule := rateLimitRule{group: "test", burst: 2, window: time.Minute}
	now := time.Now()
	for i := 0; i < 2; i++ {
		if allowed, _ := limiter.allow("key", rule, now); !allowed {
			t.Fatalf("request %d unexpectedly blocked", i+1)
		}
	}
	if allowed, retry := limiter.allow("key", rule, now); allowed || retry <= 0 {
		t.Fatalf("exhausted bucket allowed=%v retry=%s", allowed, retry)
	}
	if allowed, _ := limiter.allow("key", rule, now.Add(30*time.Second)); !allowed {
		t.Fatal("bucket did not refill after half the window")
	}
}

func TestRateLimiterConcurrentBurst(t *testing.T) {
	limiter := newRateLimiter()
	rule := rateLimitRule{group: "test", burst: 10, window: time.Minute}
	now := time.Now()
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := limiter.allow("shared", rule, now); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 10 {
		t.Fatalf("allowed %d concurrent requests, want 10", got)
	}
}
