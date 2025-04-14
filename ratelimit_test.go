/*
Copyright © 2025  M.Watermann, 10247 Berlin, Germany

	    All rights reserved
	EMail : <support@mwat.de>
*/
package ratelimit

//lint:file-ignore ST1017 - I prefer Yoda conditions

import (
	"net/http"
	"testing"
)

func Test_fastIPHash(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want uint
	}{
		{"	0", "0.0.0.0", 21},
		{"	1", "192.168.0.1", 93},
		{"	2", "192.168.1.1", 28},
		{"	3", "192.168.1.3", 48},
		{"	4", "10.10.0.1", 49},
		{"	5", "1.2.3.4", 196},
		{"	IPv6", "2001:db8::1", 88},

		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fastIPHash(tt.ip); got != tt.want {
				t.Errorf("fastIPHash() = %v, want %v", got, tt.want)
			}
		})
	}
} // Test_fastIPHash()

func Benchmark_fastIPHash(b *testing.B) {
	// runtime.GOMAXPROCS(1)

	tests := []struct {
		name string
		ip   string
		want uint
	}{
		{"	0", "0.0.0.0", 21},
		{"	1", "192.168.0.1", 93},
		{"	2", "192.168.1.1", 28},
		{"	3", "192.168.1.3", 48},
		{"	4", "10.10.0.1", 49},
		{"	5", "1.2.3.4", 196},
		{"	IPv6", "2001:db8::1", 88},
		// TODO: Add test cases.
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for _, tt := range tests {
			if got := fastIPHash(tt.ip); got != tt.want {
				continue
			}
		}
	}
} // Benchmark_getHashSum()

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

func Test_getHashFnv(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want uint
	}{
		{"	0", "0.0.0.0", 137},
		{"	1", "192.168.0.1", 131},
		{"	2", "192.168.1.1", 36},
		{"	3", "192.168.1.3", 138},
		{"	4", "10.10.0.1", 52},
		{"	5", "1.2.3.4", 229},
		{"	IPv6", "2001:db8::1", 231},

		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getHashFnv(tt.ip); got != tt.want {
				t.Errorf("getHashFnv() = %v, want %v", got, tt.want)
			}
		})
	}
} // Test_getHashFnv()

func Benchmark_getHashFnv(b *testing.B) {
	// runtime.GOMAXPROCS(1)

	tests := []struct {
		name string
		ip   string
		want uint
	}{
		{"	0", "0.0.0.0", 137},
		{"	1", "192.168.0.1", 131},
		{"	2", "192.168.1.1", 36},
		{"	3", "192.168.1.3", 138},
		{"	4", "10.10.0.1", 52},
		{"	5", "1.2.3.4", 229},
		{"	IPv6", "2001:db8::1", 231},

		// TODO: Add test cases.
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for _, tt := range tests {
			if got := getHashFnv(tt.ip); got != tt.want {
				continue
			}
		}
	}
} // Benchmark_getHashFnv()

func Test_getHashSum(t *testing.T) {

	tests := []struct {
		name string
		ip   string
		want uint
	}{
		{"	0", "0.0.0.0", 74},
		{"	1", "192.168.0.1", 38},
		{"	2", "192.168.1.1", 39},
		{"	3", "192.168.1.3", 41},
		{"	4", "10.10.0.1", 173},
		{"	5", "1.2.3.4", 84},
		{"	IPv6", "2001:db8::1", 160},

		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getHashSum(tt.ip); got != tt.want {
				t.Errorf("getHashSum() = %v, want %v", got, tt.want)
			}
		})
	}
} // Test_getHashSum()

func Benchmark_getHashSum(b *testing.B) {
	// runtime.GOMAXPROCS(1)

	tests := []struct {
		name string
		ip   string
		want uint
	}{
		{"	0", "0.0.0.0", 74},
		{"	1", "192.168.0.1", 38},
		{"	2", "192.168.1.1", 39},
		{"	3", "192.168.1.3", 41},
		{"	4", "10.10.0.1", 173},
		{"	5", "1.2.3.4", 84},
		{"	IPv6", "2001:db8::1", 160},

		// TODO: Add test cases.
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for _, tt := range tests {
			if got := getHashSum(tt.ip); got != tt.want {
				continue
			}
		}
	}
} // Benchmark_getHashSum()

/* _EoF_ */
