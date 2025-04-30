/*
Copyright © 2025  M.Watermann, 10247 Berlin, Germany

	    All rights reserved
	EMail : <support@mwat.de>
*/
package ratelimit

//lint:file-ignore ST1017 - I prefer Yoda conditions

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func Test_getClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		wantIP     string
		wantErr    bool
	}{
		{
			name:       "Valid IPv4",
			remoteAddr: "192.168.1.1:8080",
			headers:    map[string]string{},
			wantIP:     "192.168.1.1",
			wantErr:    false,
		},
		{
			name:       "Valid IPv6",
			remoteAddr: "[2001:db8::1]:8080",
			headers:    map[string]string{},
			wantIP:     "2001:db8::1",
			wantErr:    false,
		},
		{
			name:       "Valid X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
			},
			wantIP:  "203.0.113.195",
			wantErr: false,
		},
		{
			name:       "Valid X-Forwarded-For multiple IPs",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195, 70.41.3.18, 150.172.238.178",
			},
			wantIP:  "203.0.113.195",
			wantErr: false,
		},
		{
			name:       "Invalid RemoteAddr",
			remoteAddr: "invalid:8080",
			headers:    map[string]string{},
			wantIP:     "",
			wantErr:    true,
		},
		{
			name:       "Invalid X-Forwarded-For",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "invalid-ip",
			},
			wantIP:  "10.0.0.1",
			wantErr: false,
		},
		{
			name:       "Empty X-Forwarded-For",
			remoteAddr: "10.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "",
			},
			wantIP:  "10.0.0.1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			if nil != err {
				t.Fatalf("Failed to create request: %v", err)
			}

			req.RemoteAddr = tt.remoteAddr
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			gotIP, err := getClientIP(req)
			if (nil != err) != tt.wantErr {
				t.Errorf("getClientIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotIP != tt.wantIP {
				t.Errorf("getClientIP() = %v, want %v", gotIP, tt.wantIP)
			}
		})
	}
} // Test_getClientIP()

func Test_isAllowed(t *testing.T) {
	tests := []struct {
		name           string
		maxRequests    uint
		windowDuration time.Duration
		scenario       func(*tShardedLimiter) bool
	}{
		{
			name:           "First request is always allowed",
			maxRequests:    1,
			windowDuration: time.Second,
			scenario: func(sl *tShardedLimiter) bool {
				return sl.isAllowed("192.168.1.1")
			},
		},
		{
			name:           "Second request within limits",
			maxRequests:    2,
			windowDuration: time.Second,
			scenario: func(sl *tShardedLimiter) bool {
				sl.isAllowed("192.168.1.2")        // First request
				return sl.isAllowed("192.168.1.2") // Second request
			},
		},
		{
			name:           "Request exceeding limit is blocked",
			maxRequests:    1,
			windowDuration: time.Second * 30,
			scenario: func(sl *tShardedLimiter) bool {
				sl.isAllowed("192.168.1.3")        // First request
				return sl.isAllowed("192.168.1.3") // Second request should be blocked
			},
		},
		{
			name:           "Different IPs don't affect each other",
			maxRequests:    1,
			windowDuration: time.Second,
			scenario: func(sl *tShardedLimiter) bool {
				sl.isAllowed("192.168.1.4")        // Max out first IP
				return sl.isAllowed("192.168.1.5") // Different IP should be allowed
			},
		},
		{
			name:           "Requests allowed after window expires",
			maxRequests:    1,
			windowDuration: 10 * time.Millisecond,
			scenario: func(sl *tShardedLimiter) bool {
				sl.isAllowed("192.168.1.6")        // First request
				time.Sleep(20 * time.Millisecond)  // Wait for window to expire
				return sl.isAllowed("192.168.1.6") // Should be allowed in new window
			},
		},
		{
			name:           "Multiple requests within larger window",
			maxRequests:    3,
			windowDuration: time.Second,
			scenario: func(sl *tShardedLimiter) bool {
				ip := "192.168.1.7"
				sl.isAllowed(ip)        // First request
				sl.isAllowed(ip)        // Second request
				return sl.isAllowed(ip) // Third request should be allowed
			},
		},
		{
			name:           "IPv6 address handling",
			maxRequests:    1,
			windowDuration: time.Second,
			scenario: func(sl *tShardedLimiter) bool {
				ip := "2001:db8::1"
				sl.isAllowed(ip)         // First request
				return !sl.isAllowed(ip) // Second request should be blocked
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := newShardedLimiter(tt.maxRequests, tt.windowDuration)

			result := tt.scenario(limiter)

			switch tt.name {
			case "First request is always allowed",
				"Second request within limits",
				"Different IPs don't affect each other",
				"Requests allowed after window expires",
				"Multiple requests within larger window":
				if !result {
					t.Errorf("Expected request to be allowed, but it was blocked")
				}
			case "Request exceeding limit is blocked":
				if result {
					t.Errorf("Expected request to be blocked, but it was allowed")
				}
			case "IPv6 address handling":
				if !result {
					t.Errorf("IPv6 rate limiting not working as expected")
				}
			}

			// Verify metrics
			metrics := limiter.getMetrics()
			if 0 == metrics.TotalRequests {
				t.Error("TotalRequests metric should be greater than 0")
			}
		})
	}
} // Test_isAllowed()

func Test_isAllowed_Concurrent(t *testing.T) {
	limiter := newShardedLimiter(100, time.Second)
	numGoroutines := 10
	requestsPerGoroutine := 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := time.Now()

	for i := range numGoroutines {
		go func(routineID int) {
			defer wg.Done()
			ip := fmt.Sprintf("192.168.1.%d", routineID)

			for range requestsPerGoroutine {
				limiter.isAllowed(ip)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	metrics := limiter.getMetrics()
	expectedRequests := uint64(numGoroutines * requestsPerGoroutine)

	if expectedRequests != metrics.TotalRequests {
		t.Errorf("Expected %d total requests, got %d", expectedRequests, metrics.TotalRequests)
	}

	t.Logf("Concurrent test completed in %v", duration)
} // Test_isAllowed_Concurrent()

func Test_getShard(t *testing.T) {
	limiter := newShardedLimiter(100, time.Second)

	tests := []struct {
		name       string
		ip         string
		wantSameAs string
	}{
		{
			name:       "Same IP returns same shard",
			ip:         "192.168.1.1",
			wantSameAs: "192.168.1.1",
		},
		{
			name:       "Different IPs likely return different shards",
			ip:         "10.0.0.1",
			wantSameAs: "10.0.0.1",
		},
		{
			name:       "IPv6 address handling",
			ip:         "2001:db8::1",
			wantSameAs: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shard1 := limiter.getShard(tt.ip)
			shard2 := limiter.getShard(tt.wantSameAs)

			if shard1 != shard2 {
				t.Errorf("getShard() returned different shards for same IP")
			}

			// Verify shard is within bounds
			found := false
			for _, s := range limiter.shards {
				if s == shard1 {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("getShard() returned shard not in limiter.shards")
			}
		})
	}

	// Test different IPs get distributed
	shardMap := make(map[*tSlidingWindowShard]bool)
	for i := 1; i <= 50; i++ {
		ip := fmt.Sprintf("192.168.%d.%d", i%256, i%256)
		shard := limiter.getShard(ip)
		shardMap[shard] = true
	}

	// We should have multiple shards used (not just one)
	if len(shardMap) < 2 {
		t.Errorf("getShard() doesn't seem to distribute IPs: only %d shards used", len(shardMap))
	}
} // Test_getShard()

func Test_Wrap(t *testing.T) {
	tests := []struct {
		name           string
		maxRequests    uint
		windowDuration time.Duration
		scenario       func(http.Handler, TMetricsFunc) bool
	}{
		{
			name:           "No rate limiting when maxRequests is 0",
			maxRequests:    0,
			windowDuration: time.Second,
			scenario: func(handler http.Handler, getMetrics TMetricsFunc) bool {
				return (nil == getMetrics) // Should return nil metrics function
			},
		},
		{
			name:           "Rate limiting is applied",
			maxRequests:    1,
			windowDuration: time.Second,
			scenario: func(handler http.Handler, getMetrics TMetricsFunc) bool {
				// Create test request
				req, _ := http.NewRequest("GET", "http://example.com", nil)
				req.RemoteAddr = "192.168.1.1:8080"

				// First request should be allowed
				rr1 := httptest.NewRecorder()
				handler.ServeHTTP(rr1, req)

				// Second request should be blocked
				rr2 := httptest.NewRecorder()
				handler.ServeHTTP(rr2, req)

				// Check metrics
				metrics := getMetrics()

				return (http.StatusOK == rr1.Code) &&
					(http.StatusTooManyRequests == rr2.Code) &&
					(2 == metrics.TotalRequests) &&
					(1 == metrics.BlockedRequests)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple handler that returns 200 OK
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Apply the Wrap function
			handler, getMetrics := Wrap(nextHandler, tt.maxRequests, tt.windowDuration)

			// Run the test scenario
			result := tt.scenario(handler, getMetrics)

			if !result {
				t.Errorf("Test failed: %s", tt.name)
			}
		})
	}
} // Test_Wrap()

/* _EoF_ */
