package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/bjschafer/print-dis/internal/response"
)

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	buckets map[string]*bucket
	mutex   sync.RWMutex
	
	// Configuration
	maxRequests int           // Maximum requests per window
	window      time.Duration // Time window
	cleanupTick time.Duration // How often to clean up old buckets
}

type bucket struct {
	tokens    int
	lastSeen  time.Time
	resetTime time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets:     make(map[string]*bucket),
		maxRequests: maxRequests,
		window:      window,
		cleanupTick: window * 2, // Clean up buckets twice per window
	}
	
	// Start cleanup goroutine
	go rl.cleanup()
	
	return rl
}

// Allow checks if a request from the given key should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	
	// Get or create bucket for this key
	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{
			tokens:    rl.maxRequests - 1, // Consume one token for this request
			lastSeen:  now,
			resetTime: now.Add(rl.window),
		}
		rl.buckets[key] = b
		return true
	}
	
	// Update last seen
	b.lastSeen = now
	
	// Check if window has reset
	if now.After(b.resetTime) {
		b.tokens = rl.maxRequests - 1 // Reset and consume one token
		b.resetTime = now.Add(rl.window)
		return true
	}
	
	// Check if tokens available
	if b.tokens > 0 {
		b.tokens--
		return true
	}
	
	return false
}

// cleanup removes old buckets to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupTick)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mutex.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window * 2) // Remove buckets not seen for 2 windows
		
		for key, bucket := range rl.buckets {
			if bucket.lastSeen.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mutex.Unlock()
	}
}

// RateLimitByIP creates a rate limiting middleware that limits by IP address
func RateLimitByIP(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(maxRequests, window)
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client IP
			ip := getClientIP(r)
			
			if !limiter.Allow(ip) {
				response.WriteErrorResponse(w, http.StatusTooManyRequests, response.BadRequest, 
					"Rate limit exceeded. Please try again later.", "")
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// AuthRateLimit creates a stricter rate limit for authentication endpoints
func AuthRateLimit() func(http.Handler) http.Handler {
	// More restrictive: 5 requests per minute for auth endpoints
	return RateLimitByIP(5, time.Minute)
}

// APIRateLimit creates a general rate limit for API endpoints
func APIRateLimit() func(http.Handler) http.Handler {
	// General API: 100 requests per minute
	return RateLimitByIP(100, time.Minute)
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		return xff
	}
	
	// Check X-Real-IP header (for proxies)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colonIndex := len(ip) - 1; colonIndex > 0 {
		for i := colonIndex; i >= 0; i-- {
			if ip[i] == ':' {
				return ip[:i]
			}
		}
	}
	
	return ip
}