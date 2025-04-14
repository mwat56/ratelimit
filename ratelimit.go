/*
Copyright © 2025  M.Watermann, 10247 Berlin, Germany

	    All rights reserved
	EMail : <support@mwat.de>
*/
package ratelimit

//lint:file-ignore ST1017 - I prefer Yoda conditions

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type (
	tSlidingWindowCounter struct {
		mtx          sync.Mutex
		prevCount    int
		currentCount int
		windowStart  time.Time
	}

	tClientList map[string]*tSlidingWindowCounter

	tSlidingWindowLimiter struct {
		sync.Mutex
		clients         tClientList
		maxRequests     int
		windowDuration  time.Duration
		cleanupInterval time.Duration
	}
)

func (l *tSlidingWindowLimiter) cleanup() {
	l.Lock()
	defer l.Unlock()

	threshold := time.Now().UTC().Add(-l.windowDuration * 2)
	for ip, counter := range l.clients {
		func() {
			counter.mtx.Lock()
			defer counter.mtx.Unlock()

			// Remove clients that haven't made any requests
			// in the last two windows:
			if counter.windowStart.Before(threshold) && counter.currentCount == 0 {
				delete(l.clients, ip)
			}
		}()
	}
} // cleanup()

func (l *tSlidingWindowLimiter) startCleanup() {
	ticker := time.NewTicker(l.cleanupInterval)
	for range ticker.C {
		l.cleanup()
	}
} // startCleanup()

// `cleanIP()` validates and formats an IP address string.
//
// The function handles both IPv4 and IPv6 addresses, ensuring they are
// in a consistent format.
//
// Parameters:
//   - `aIP`: The IP address to clean and validate
//
// Returns:
//   - `string`: A cleaned and validated IP address, or empty string if invalid
func cleanIP(aIP string) string {
	// Remove any brackets from IPv6 addresses
	aIP = strings.Trim(aIP, "[]")

	// Parse IP address
	netIP := net.ParseIP(aIP)
	if nil == netIP {
		return ""
	}

	// Convert to consistent format
	if ipv4 := netIP.To4(); ipv4 != nil {
		return ipv4.String()
	}

	return netIP.String()
} // cleanIP()

// `getClientIP()` extracts and validates the client's IP address from
// an HTTP request.
//
// The function handles both IPv4 and IPv6 addresses and properly processes
// `X-Forwarded-For` headers in proxy chains. It follows these steps to
// determine the client IP:
// 1. Check `X-Forwarded-For` header
// 2. Extract the leftmost valid IP (original client)
// 3. Fall back to `RemoteAddr` if no valid IP is found
// 4. Clean and validate the IP address
//
// Parameters:
//   - `aRequest`: The incoming HTTP request containing client information.
//
// Returns:
//   - `string`: A validated client IP address
//   - `error`: Error if no valid IP address could be determined
func getClientIP(aRequest *http.Request) (string, error) {
	// First try X-Forwarded-For header
	if xff := aRequest.Header.Get("X-Forwarded-For"); "" != xff {
		// Split IPs and get the original client IP (leftmost)
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			// Clean the IP string
			ip = strings.TrimSpace(ip)
			if validIP := cleanIP(ip); "" != validIP {
				return validIP, nil
			}
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(aRequest.RemoteAddr)
	if err != nil {
		// Try RemoteAddr directly in case it's just an IP
		if validIP := cleanIP(aRequest.RemoteAddr); "" != validIP {
			return validIP, nil
		}
		return "", fmt.Errorf("invalid RemoteAddr: %v", err)
	}

	if validIP := cleanIP(host); "" != validIP {
		return validIP, nil
	}

	return "", fmt.Errorf("no valid IP address found")
} // getClientIP()

// newLimiter creates a new rate limiter with automatic cleanup
func newLimiter(aMaxReq int, aDuration time.Duration) *tSlidingWindowLimiter {
	limiter := &tSlidingWindowLimiter{
		clients:         make(tClientList),
		maxRequests:     aMaxReq,
		windowDuration:  aDuration,
		cleanupInterval: aDuration * 2,
	}

	go limiter.startCleanup()

	return limiter
} // newLimiter()

// ---------------------------------------------------------------------------

// `Wrap()` creates a new rate limiting middleware handler.
// It uses a sliding window algorithm to limit requests per client IP.
//
// Parameters:
//   - `aNext`: The next handler in the middleware chain.
//   - `aMaxReq`: Maximum number of requests allowed per window.
//   - `aDuration`: The time window duration.
//
// Returns:
//   - `http.Handler`: A new handler that implements rate limiting
func Wrap(aNext http.Handler, aMaxReq int, aDuration time.Duration) http.Handler {
	limiter := newLimiter(aMaxReq, aDuration)

	return http.HandlerFunc(func(aWriter http.ResponseWriter, aRequest *http.Request) {
		// Get and validate client IP
		clientIP, err := getClientIP(aRequest)
		if err != nil {
			http.Error(aWriter, "Forbidden - Invalid IP", http.StatusForbidden)
			return
		}

		limiter.Lock()
		defer limiter.Unlock()

		now := time.Now().UTC() // Use UTC to avoid DST issues
		counter, exists := limiter.clients[clientIP]
		if !exists {
			counter = &tSlidingWindowCounter{
				windowStart: now,
			}
			limiter.clients[clientIP] = counter
		}

		counter.mtx.Lock()
		defer counter.mtx.Unlock()

		elapsed := now.Sub(counter.windowStart)
		if elapsed > limiter.windowDuration {
			// Window has expired, reset counts
			counter.prevCount = counter.currentCount
			counter.currentCount = 0
			counter.windowStart = now
			elapsed = 0
		}

		// Calculate the weighted request count
		remainingWindow := limiter.windowDuration - elapsed
		prevWeight := float64(remainingWindow) / float64(limiter.windowDuration)
		weightedCount := int(float64(counter.prevCount)*prevWeight) + counter.currentCount + 1

		// Check if the request would exceed the rate limit
		if weightedCount > limiter.maxRequests {
			http.Error(aWriter, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		counter.currentCount++
		aNext.ServeHTTP(aWriter, aRequest)
	})
} // Wrap()

/* _EoF_ */
