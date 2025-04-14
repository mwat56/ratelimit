# RateLimit

[![golang](https://img.shields.io/badge/Language-Go-green.svg)](https://golang.org/)
[![GoDoc](https://godoc.org/github.com/mwat56/ratelimit?status.svg)](https://godoc.org/github.com/mwat56/ratelimit)
[![Go Report](https://goreportcard.com/badge/github.com/mwat56/ratelimit)](https://goreportcard.com/report/github.com/mwat56/ratelimit)
[![Issues](https://img.shields.io/github/issues/mwat56/ratelimit.svg)](https://github.com/mwat56/ratelimit/issues?q=is%3Aopen+is%3Aissue)
[![Size](https://img.shields.io/github/repo-size/mwat56/ratelimit.svg)](https://github.com/mwat56/ratelimit/)
[![Tag](https://img.shields.io/github/tag/mwat56/ratelimit.svg)](https://github.com/mwat56/ratelimit/tags)
[![View examples](https://img.shields.io/badge/learn%20by-examples-0077b3.svg)](https://github.com/mwat56/ratelimit/blob/main/_demo/demo.go)
[![License](https://img.shields.io/github/mwat56/ratelimit.svg)](https://github.com/mwat56/ratelimit/blob/main/LICENSE)

- [RateLimit](#ratelimit)
	- [Purpose](#purpose)
		- [Key Features](#key-features)
		- [Basic Concept](#basic-concept)
		- [Implementation in this package](#implementation-in-this-package)
			- [Sharding Architecture:](#sharding-architecture)
			- [Request Tracking:](#request-tracking)
			- [Rate Limiting Logic](#rate-limiting-logic)
		- [How it Works](#how-it-works)
		- [Advantages](#advantages)
	- [Installation](#installation)
	- [Usage](#usage)
	- [Libraries](#libraries)
	- [Licence](#licence)

----

## Purpose

This package implements _rate limiting middleware_ for HTTP servers. It provides functionality to control the number of requests from individual IP addresses within a specified time window.

A _sliding window rate limiter_ is a rate limiting algorithm that provides a smoother, more accurate way to control request rates compared to fixed window approaches.

### Key Features

- Uses a sliding window algorithm for rate limiting.
- Handles both IPv4 and IPv6 addresses.
- Properly processes X-Forwarded-For headers for proxy chains.
- Thread-safe implementation using mutexes.

### Basic Concept

- Instead of using fixed time windows (e.g., exactly from 2:00 to 3:00), the window "slides" with time.
- Each request is evaluated against a window that ends at the current time and starts at (current time - window duration).
- Provides more accurate rate limiting by avoiding edge cases at window boundaries.

### Implementation in this package

The rate limiter in this package uses a sharded sliding window algorithm.

#### Sharding Architecture:

	type tShardedLimiter struct {
		shards          [256]*tSlidingWindowShard // fixed size array of shards
		maxRequests     int                       // maximum requests per window
		windowDuration  time.Duration             // duration of the sliding window
		cleanupInterval time.Duration             // interval between cleanup runs
		metrics         TMetrics                  // metrics for rate limiting
	}

The limiter distributes clients across 256 shards to reduce lock contention.

#### Request Tracking:

	type (
		tSlidingWindowCounter struct {
			sync.Mutex             // protects counter fields
			prevCount    int       // requests in previous window
			currentCount int       // requests in current window
			windowStart  time.Time // start time of current window
		}
	)

Each client IP gets a counter that tracks:

- Requests in the previous window
- Requests in the current window
- Start time of the current window

#### Rate Limiting Logic

When a request comes in:

1. If it's a new IP, create a counter and allow the request.
2. If the window has expired, reset the counter.
3. Otherwise, increment the counter and check against the limit.
4. Cleanup: The system automatically cleans up inactive clients:

		func (sl *tShardedLimiter) cleanup() {
			threshold := time.Now().UTC().Add(-sl.windowDuration * 2)
			for _, sws := range sl.shards {
				sws.cleanShard(threshold)
			}
		}

This implementation provides efficient rate limiting with:

- Thread-safe operation through sharding and locks,
- Automatic cleanup of inactive clients,
- Support for both IPv4 and IPv6 addresses,
- metrics for monitoring,
- proper handling of both IPv4 and IPv6 addresses,
- support for proxy chains via X-Forwarded-For headers.

### How it Works

Let's say we have a 60-second window limit of 100 requests. Here's how the calculation works:

	Window Duration: 60 seconds
	Current Time: 2:00:45
	Window Start: 2:00:00
	Previous Window Count: 80
	Current Window Count: 30
	Elapsed Time: 45 seconds
	Remaining Time: 15 seconds

	Weight calculation:
	- Remaining = 15 seconds = 25% of window
	- Previous window weight = 0.25 (25%)
	- Weighted count = (80 * 0.25) + 30 = 50

**Visual representation:**

	Previous Window     Current Window
	[    80 reqs    ][    30 reqs    ]
	|               |                |
	1:59:00        2:00:00         2:00:45
					|----- 45s -----|
					|---- 15s ---|
					(remaining time)

### Advantages

- Smoother rate limiting without sharp cutoffs.
- More accurate request counting.
- Prevents request spikes at window boundaries.
- Better handles continuous traffic patterns.

The sliding window approach provides a more sophisticated rate limiting solution than other methods that

- prevents traffic spikes at window boundaries,
- provides more accurate rate limiting,
- better handles real-world traffic patterns,
- maintains a smoother request distribution over time.

This makes it particularly suitable for APIs and web services where consistent request handling is important.

## Installation

You can use `Go` to install this package for you:

    go get -u github.com/mwat56/ratelimit

## Usage

To include the rate limiting provided by this package you just call the `Wrap()` function as shown here:

	import "github.com/mwat56/ratelimit"
	// ...

	func main() {
		// ...
		// system setup etc.

		maxRequests := 1000 // max requests per minute
		windowDuration := time.Minute // window duration
		// These values should probably come from some
		// configuration file or commandline option.

		pageHandler := http.NewServeMux() // or your own page handling provider
		pageHandler.HandleFunc("/", myHandler) // dito

		// Create the rate-limited handler and get the metrics function
		handler, getMetrics := ratelimit.Wrap(pageHandler, maxRequests, windowDuration)
		//                     ^^^^^^^^^^^^^^^

		// Start a goroutine to periodically log metrics
		go func() {
			for range time.NewTicker(time.Minute).C {
				metrics := getMetrics()
				log.Printf("Rate Limiter Metrics:\n"+
					"Total Requests: %d\n"+
					"Blocked Requests: %d\n"+
					"Active Clients: %d\n"+
					"Cleanup Interval: %v\n",
					metrics.TotalRequests,
					metrics.BlockedRequests,
					metrics.ActiveClients,
					metrics.CleanupDuration)
			}
		}()

		// Expose metrics endpoint for monitoring systems
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			metrics := getMetrics()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(metrics)
		})

		server := http.Server{
			Addr:    "127.0.0.1:8080",
			Handler: handler, // <== here is the rate-limited handler
		}

		if err := server.ListenAndServe(); nil != err {
			log.Fatalf("%s: %v", os.Args[0], err)
		}
	} // main()

The `Wrap()` function creates a new handler and returns it along with a function to retrieve metrics:

- `http.Handler`: A new handler that implements rate limiting
- `func() TMetrics`: A function that returns usage metrics for monitoring.

The returned handler implements rate limiting by:

1. extracting the client IP,
2. checking if the request is allowed,
3. either allowing the request to proceed or sending a `429 (Too Many Requests)` error to the client.

In the example above, the implementation provides three ways to monitor your rate limiter:

- Periodic logging of metrics,
- HTTP endpoint for monitoring systems,
- on-demand access to metrics through the getter function.

## Libraries

No external libraries were used building `ratelimit`.

## Licence

        Copyright © 2025 M.Watermann, 10247 Berlin, Germany
                        All rights reserved
                    EMail : <support@mwat.de>

> This program is free software; you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation; either version 3 of the License, or (at your option) any later version.
>
> This software is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
>
> You should have received a copy of the GNU General Public License along with this program. If not, see the [GNU General Public License](http://www.gnu.org/licenses/gpl.html) for details.

----
[![GFDL](https://www.gnu.org/graphics/gfdl-logo-tiny.png)](http://www.gnu.org/copyleft/fdl.html)
