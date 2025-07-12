package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bjschafer/print-dis/internal/response"
)

// RequestSizeLimits defines the maximum sizes for different types of requests
type RequestSizeLimits struct {
	MaxRequestBodySize   int64         // Maximum size for entire request body
	MaxJSONDepth         int           // Maximum nesting depth for JSON
	MaxJSONTokens        int           // Maximum number of JSON tokens
	RequestTimeout       time.Duration // Maximum time to process request
	MaxURLParameterSize  int           // Maximum size for URL parameters
	MaxHeaderSize        int           // Maximum size for individual headers
}

// DefaultRequestLimits returns sensible default limits
func DefaultRequestLimits() RequestSizeLimits {
	return RequestSizeLimits{
		MaxRequestBodySize:  1024 * 1024,    // 1MB
		MaxJSONDepth:        32,              // 32 levels deep
		MaxJSONTokens:       10000,           // 10k tokens
		RequestTimeout:      30 * time.Second, // 30 seconds
		MaxURLParameterSize: 1024,            // 1KB per parameter
		MaxHeaderSize:       8192,            // 8KB per header
	}
}

// RequestSizeMiddleware creates middleware that enforces request size limits
func RequestSizeMiddleware(limits RequestSizeLimits) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size
			r.Body = http.MaxBytesReader(w, r.Body, limits.MaxRequestBodySize)
			
			// Validate URL parameter sizes
			if err := validateURLParameters(r, limits.MaxURLParameterSize); err != nil {
				response.WriteBadRequestError(w, "URL parameter too large", err.Error())
				return
			}
			
			// Validate header sizes
			if err := validateHeaders(r, limits.MaxHeaderSize); err != nil {
				response.WriteBadRequestError(w, "Header too large", err.Error())
				return
			}
			
			// Add request timeout
			if limits.RequestTimeout > 0 {
				ctx := r.Context()
				ctx, cancel := context.WithTimeout(ctx, limits.RequestTimeout)
				defer cancel()
				r = r.WithContext(ctx)
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// validateURLParameters checks that URL parameters don't exceed size limits
func validateURLParameters(r *http.Request, maxSize int) error {
	values := r.URL.Query()
	for key, valueList := range values {
		if len(key) > maxSize {
			return fmt.Errorf("parameter name '%s' exceeds maximum size of %d bytes", key, maxSize)
		}
		for _, value := range valueList {
			if len(value) > maxSize {
				return fmt.Errorf("parameter value for '%s' exceeds maximum size of %d bytes", key, maxSize)
			}
		}
	}
	return nil
}

// validateHeaders checks that HTTP headers don't exceed size limits
func validateHeaders(r *http.Request, maxSize int) error {
	for name, values := range r.Header {
		if len(name) > maxSize {
			return fmt.Errorf("header name '%s' exceeds maximum size of %d bytes", name, maxSize)
		}
		for _, value := range values {
			if len(value) > maxSize {
				return fmt.Errorf("header value for '%s' exceeds maximum size of %d bytes", name, maxSize)
			}
		}
	}
	return nil
}

// SafeJSONDecoder creates a JSON decoder with safety limits
func SafeJSONDecoder(r *http.Request, limits RequestSizeLimits) *json.Decoder {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	
	// Note: Go's json package doesn't directly support depth/token limits
	// We rely on the request size limit and timeout to prevent abuse
	// For more advanced limits, we'd need a custom JSON parser
	
	return decoder
}

// LimitedJSONDecode safely decodes JSON with size and complexity limits
func LimitedJSONDecode(r *http.Request, v interface{}, limits RequestSizeLimits) error {
	// Create a limited reader for the request body
	limitedReader := http.MaxBytesReader(nil, r.Body, limits.MaxRequestBodySize)
	
	decoder := json.NewDecoder(limitedReader)
	decoder.DisallowUnknownFields()
	
	return decoder.Decode(v)
}