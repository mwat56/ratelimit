/*
Copyright © 2025  M.Watermann, 10247 Berlin, Germany

	    All rights reserved
	EMail : <support@mwat.de>
*/
package ratelimit

//lint:file-ignore ST1017 - I prefer Yoda conditions

import (
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type (
	// `tSlidingWindowCounter` tracks request counts within a time
	// window for a single client IP address.
	tSlidingWindowCounter struct {
		sync.Mutex             // protects counter fields
		prevCount    int       // requests in previous window
		currentCount int       // requests in current window
		windowStart  time.Time // start time of current window
	}

	// `tClientList` maps IP addresses to their respective request
	// counters.
	tClientList map[string]*tSlidingWindowCounter

	// `tSlidingWindowShard` represents a single shard of the rate
	// limiter, managing a subset of client IPs.
	tSlidingWindowShard struct {
		sync.Mutex             // protects clients map
		clients    tClientList // IP-to-counter map  for this shard
	}

	// `tShardedLimiter` implements a sharded rate limiter that distributes
	// client IPs across multiple shards to reduce lock contention.
	tShardedLimiter struct {
		shards          [256]*tSlidingWindowShard // fixed size array of shards
		maxRequests     int                       // maximum requests per window
		windowDuration  time.Duration             // duration of the sliding window
		cleanupInterval time.Duration             // interval between cleanup runs
	}
)

// ---------------------------------------------------------------------------
// `tSlidingWindowShard` methods:

// `cleanShard()` removes inactive clients from the shard that haven't
// made requests in the specified threshold period.
//
// Parameters:
//   - `aThreshold`: The threshold time before which clients are considered inactive.
func (sws *tSlidingWindowShard) cleanShard(aThreshold time.Time) {
	del := func(aCounter *tSlidingWindowCounter, aIP string) {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Recovered from panic:", r)
			}
		}()

		aCounter.Lock()
		defer aCounter.Unlock()

		// Remove clients that haven't made any requests
		// in the last two windows:
		if aCounter.windowStart.Before(aThreshold) {
			// &&(0 == aCounter.currentCount) {
			delete(sws.clients, aIP)
		}
	}

	sws.Lock()
	defer sws.Unlock()

	for ip, counter := range sws.clients {
		del(counter, ip)
	}
} // cleanShard()

// ---------------------------------------------------------------------------
// `tShardedLimiter` methods:

// `cleanup()` performs maintenance on all shards by removing inactive clients.
func (sl *tShardedLimiter) cleanup() {
	threshold := time.Now().UTC().Add(-sl.windowDuration * 2)

	for _, sws := range sl.shards {
		sws.cleanShard(threshold)
	}
} // cleanup()

// `cleanupStart()` initiates a background goroutine that periodically
// cleans up inactive clients from all shards.
func (sl *tShardedLimiter) cleanupStart() {
	ticker := time.NewTicker(sl.cleanupInterval)

	go func() {
		for range ticker.C {
			sl.cleanup()
		}
	}()
} // cleanupStart()

// `getShard()` returns the appropriate shard for a given IP address
// using a hash-based distribution.
//
// Parameters:
//   - `aIP`: The IP address of the client making the request.
//
// Returns:
//   - `*tSlidingWindowShard`: The shard holding the given IP address.
func (sl *tShardedLimiter) getShard(aIP string) *tSlidingWindowShard {
	// Simple hash function for IP-based sharding
	sum := 0
	for i := 0; i < len(aIP); i++ {
		sum += int(aIP[i])
	}

	return sl.shards[sum%256]
} // getShard()

// `isAllowed()` checks if a request from the given IP address is
// allowed based on the rate limiting rules.
//
// Parameters:
//   - `aIP`: The IP address of the client making the request.
//
// Returns:
//   - `bool`: Whether the request is within the rate limits.
func (sl *tShardedLimiter) isAllowed(aIP string) bool {
	shard := sl.getShard(aIP)
	shard.Lock()
	defer shard.Unlock()

	now := time.Now().UTC() // Use UTC to avoid DST issues
	counter, exists := shard.clients[aIP]
	if !exists {
		counter = &tSlidingWindowCounter{
			currentCount: 1,
			windowStart:  now,
		}
		shard.clients[aIP] = counter
		// First request is always allowed
		return true
	}

	counter.Lock()
	defer counter.Unlock()

	if sl.windowDuration < time.Since(counter.windowStart) {
		// Window has expired, reset counts
		counter.prevCount = 0
		counter.currentCount = 1
		counter.windowStart = now

		return true
	}
	counter.prevCount = counter.currentCount
	counter.currentCount++

	// Return whether the request would exceed the rate limit
	return counter.currentCount <= sl.maxRequests
} // isAllowed()

// ---------------------------------------------------------------------------
// helper functions:

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

func fastIPHash(aIP string) uint {
	ip := net.ParseIP(aIP)
	if nil == ip {
		return 0
	}

	// Fastest non-cryptographic hash (CRC-32)
	return uint(crc32.ChecksumIEEE(ip) % 256)
} // fastIPHash()

func getHashFnv(aIP string) uint {
	h := fnv.New64a()
	// Safe for digits/dots (no allocations beyond byte conversion):
	h.Write([]byte(aIP))

	return uint(h.Sum64() % 256)
} // getHashFnv()

func getHashSum(aIP string) uint {
	// Simple hash function for IP-based sharding
	sum := 0
	for i := 0; i < len(aIP); i++ {
		sum += int(aIP[i])
	}

	return uint(sum % 256)
} // getHashSum()

// ---------------------------------------------------------------------------
// constructor methods:

// `newShard()` creates a new rate limiter shard.
func newShard() *tSlidingWindowShard {
	return &tSlidingWindowShard{
		clients: make(tClientList),
	}
} // newShard()

// `newShardedLimiter()` creates a new sharded rate limiter.
func newShardedLimiter(aMaxReq int, aDuration time.Duration) *tShardedLimiter {
	result := &tShardedLimiter{
		maxRequests:     aMaxReq,
		windowDuration:  aDuration,
		cleanupInterval: aDuration * 2,
	}

	for i := range result.shards {
		result.shards[i] = newShard()
	}

	// Start the cleanup routine
	result.cleanupStart()

	return result
} // newShardedLimiter()

// ---------------------------------------------------------------------------
// exported functions:

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
	limiter := newShardedLimiter(aMaxReq, aDuration)

	return http.HandlerFunc(func(aWriter http.ResponseWriter, aRequest *http.Request) {
		// Get and validate client IP
		clientIP, err := getClientIP(aRequest)
		if nil != err {
			http.Error(aWriter, "Forbidden - Invalid IP", http.StatusForbidden)
			return
		}

		if !limiter.isAllowed(clientIP) {
			http.Error(aWriter, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		aNext.ServeHTTP(aWriter, aRequest)
	})
} // Wrap()

/* _EoF_ */
