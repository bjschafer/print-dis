package middleware

import (
	"net/http"
)

// SecurityHeaders adds security headers to HTTP responses
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Content Security Policy - prevent XSS attacks
			// Allow 'self' for scripts, styles, and images
			// 'unsafe-inline' for styles is needed for current CSS implementation
			// Consider removing 'unsafe-inline' when CSS is refactored
			csp := "default-src 'self'; " +
				"script-src 'self'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: https:; " +
				"font-src 'self'; " +
				"connect-src 'self'; " +
				"frame-ancestors 'none'; " +
				"base-uri 'self'; " +
				"form-action 'self'"
			
			w.Header().Set("Content-Security-Policy", csp)
			
			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")
			
			// Prevent clickjacking attacks
			w.Header().Set("X-Frame-Options", "DENY")
			
			// Enable XSS protection (legacy browsers)
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			
			// Prevent page from being embedded in frames on other domains
			w.Header().Set("Frame-Options", "SAMEORIGIN")
			
			// Force HTTPS in production (commented out for development)
			// w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			
			// Control referrer information
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			
			// Disable potentially dangerous features
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			
			next.ServeHTTP(w, r)
		})
	}
}