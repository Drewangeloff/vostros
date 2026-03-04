package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("key1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow("key1") {
		t.Fatal("4th request should be rate limited")
	}
	// Different key should still be allowed
	if !l.Allow("key2") {
		t.Fatal("different key should be allowed")
	}
}

func TestLimiter_WindowReset(t *testing.T) {
	l := New(1, 50*time.Millisecond)
	if !l.Allow("k") {
		t.Fatal("first request should be allowed")
	}
	if l.Allow("k") {
		t.Fatal("second request should be limited")
	}
	time.Sleep(60 * time.Millisecond)
	if !l.Allow("k") {
		t.Fatal("request after window reset should be allowed")
	}
}

func TestClientIP_FirstXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1, 10.0.0.2")
	ip := clientIP(r)
	if ip != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", ip)
	}
}

func TestClientIP_SingleXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "5.6.7.8")
	ip := clientIP(r)
	if ip != "5.6.7.8" {
		t.Fatalf("expected 5.6.7.8, got %s", ip)
	}
}

func TestClientIP_NoXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"
	ip := clientIP(r)
	if ip != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", ip)
	}
}

func TestClientIP_SpoofedXFF(t *testing.T) {
	// Attacker appends values — only the first (set by LB) should be used
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, spoofed-value")
	ip := clientIP(r)
	if ip != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", ip)
	}
}

func TestMiddleware_Returns429(t *testing.T) {
	l := New(1, time.Minute)
	handler := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request passes
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Second request blocked
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}
