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
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			req.RemoteAddr = tt.remoteAddr
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			gotIP, err := getClientIP(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("getClientIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotIP != tt.wantIP {
				t.Errorf("getClientIP() = %v, want %v", gotIP, tt.wantIP)
			}
		})
	}
} // Test_getClientIP()

/* _EoF_ */
