package validation

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bjschafer/print-dis/internal/response"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return ""
	}
	if len(ve) == 1 {
		return fmt.Sprintf("validation error: %s %s", ve[0].Field, ve[0].Message)
	}
	return fmt.Sprintf("validation errors: %d fields failed validation", len(ve))
}

// Validator provides input validation utilities
type Validator struct {
	errors ValidationErrors
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make(ValidationErrors, 0),
	}
}

// HasErrors returns true if there are validation errors
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors
func (v *Validator) Errors() ValidationErrors {
	return v.errors
}

// AddError adds a validation error
func (v *Validator) AddError(field, message string) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// ValidateRequired checks if a field is not empty
func (v *Validator) ValidateRequired(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.AddError(field, "is required")
	}
}

// ValidateLength checks if a field meets length requirements
func (v *Validator) ValidateLength(field, value string, min, max int) {
	length := utf8.RuneCountInString(value)
	if length < min {
		v.AddError(field, fmt.Sprintf("must be at least %d characters", min))
	}
	if max > 0 && length > max {
		v.AddError(field, fmt.Sprintf("must be no more than %d characters", max))
	}
}

// ValidateEmail checks if a field is a valid email
func (v *Validator) ValidateEmail(field, value string) {
	if value == "" {
		return // Allow empty emails, use ValidateRequired separately if needed
	}
	
	// RFC 5321 limit for email addresses
	if len(value) > MaxEmailLength {
		v.AddError(field, fmt.Sprintf("email address too long (max %d characters)", MaxEmailLength))
		return
	}
	
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(value) {
		v.AddError(field, "must be a valid email address")
	}
}

// ValidateURL checks if a field is a valid URL
func (v *Validator) ValidateURL(field, value string) {
	if value == "" {
		return // Allow empty URLs, use ValidateRequired separately if needed
	}
	if _, err := url.ParseRequestURI(value); err != nil {
		v.AddError(field, "must be a valid URL")
	}
}

// ValidateUsername checks if a field is a valid username
func (v *Validator) ValidateUsername(field, value string) {
	if value == "" {
		return
	}
	
	// Check length first
	if len(value) > MaxUsernameLength {
		v.AddError(field, fmt.Sprintf("username too long (max %d characters)", MaxUsernameLength))
		return
	}
	
	// Username must be 3-50 characters, alphanumeric with underscore and dash
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]{3,50}$`)
	if !usernameRegex.MatchString(value) {
		v.AddError(field, "must be 3-50 characters, alphanumeric with underscore and dash only")
	}
}

// ValidateNoHTML checks that a field doesn't contain HTML tags
func (v *Validator) ValidateNoHTML(field, value string) {
	if value == "" {
		return
	}
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	if htmlRegex.MatchString(value) {
		v.AddError(field, "cannot contain HTML tags")
	}
}

// ValidateFileURL validates that a URL is safe for file links
func (v *Validator) ValidateFileURL(field, value string) {
	if value == "" {
		return
	}
	
	// Check URL length first to prevent very long URLs
	if len(value) > MaxFileURLLength {
		v.AddError(field, fmt.Sprintf("URL too long (max %d characters)", MaxFileURLLength))
		return
	}
	
	// Check for suspicious protocols first, before parsing
	if strings.Contains(strings.ToLower(value), "javascript:") {
		v.AddError(field, "cannot contain JavaScript URLs")
		return
	}
	
	// Check for data URLs which could be used for XSS
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		v.AddError(field, "data URLs are not allowed")
		return
	}
	
	// Parse URL
	u, err := url.ParseRequestURI(value)
	if err != nil {
		v.AddError(field, "must be a valid URL")
		return
	}
	
	// Check scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		v.AddError(field, "must use HTTP or HTTPS")
		return
	}
}

// ValidateDisplayName validates display names
func (v *Validator) ValidateDisplayName(field, value string) {
	if value == "" {
		return
	}
	
	v.ValidateLength(field, value, 1, MaxDisplayNameLength)
	v.ValidateNoHTML(field, value)
	
	// Check for potentially dangerous characters
	if strings.ContainsAny(value, "<>&\"'") {
		v.AddError(field, "cannot contain HTML special characters")
	}
}

// ValidateNotes validates notes and descriptions
func (v *Validator) ValidateNotes(field, value string) {
	if value == "" {
		return
	}
	
	v.ValidateLength(field, value, 0, MaxNotesLength)
	v.ValidateNoHTML(field, value)
	
	// Check for JavaScript
	if strings.Contains(strings.ToLower(value), "javascript:") {
		v.AddError(field, "cannot contain JavaScript")
	}
}

// ValidateColor validates color names
func (v *Validator) ValidateColor(field, value string) {
	if value == "" {
		return
	}
	
	v.ValidateLength(field, value, 0, MaxColorLength)
	
	// Only allow alphanumeric characters, spaces, hyphens, and hash for hex colors
	colorRegex := regexp.MustCompile(`^[a-zA-Z0-9\s\-#]+$`)
	if !colorRegex.MatchString(value) {
		v.AddError(field, "contains invalid characters")
	}
}

// ValidateMaterial validates material names
func (v *Validator) ValidateMaterial(field, value string) {
	if value == "" {
		return
	}
	
	v.ValidateLength(field, value, 0, MaxMaterialLength)
	
	// Only allow alphanumeric characters, spaces, hyphens, plus, and periods
	materialRegex := regexp.MustCompile(`^[a-zA-Z0-9\s\-+.]+$`)
	if !materialRegex.MatchString(value) {
		v.AddError(field, "contains invalid characters")
	}
}

// SanitizeString trims whitespace and removes null bytes
func SanitizeString(s string) string {
	// Remove null bytes and control characters except whitespace
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// SanitizeDisplayName sanitizes display names for safe storage and display
func SanitizeDisplayName(s string) string {
	// First apply basic sanitization
	s = SanitizeString(s)
	
	// Remove any HTML tags
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	s = htmlRegex.ReplaceAllString(s, "")
	
	// Remove potentially dangerous characters
	s = strings.ReplaceAll(s, "<", "")
	s = strings.ReplaceAll(s, ">", "")
	s = strings.ReplaceAll(s, "&", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")
	
	return s
}

// SanitizeNotes sanitizes notes and descriptions for safe storage
func SanitizeNotes(s string) string {
	// Apply basic sanitization
	s = SanitizeString(s)
	
	// Remove HTML tags
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	s = htmlRegex.ReplaceAllString(s, "")
	
	// Remove script tags and javascript URLs
	jsRegex := regexp.MustCompile(`(?i)javascript:`)
	s = jsRegex.ReplaceAllString(s, "")
	
	return s
}

// SanitizeColor sanitizes color names for safe storage
func SanitizeColor(s string) string {
	s = SanitizeString(s)
	
	// Only allow alphanumeric characters, spaces, and common color characters
	colorRegex := regexp.MustCompile(`[^a-zA-Z0-9\s\-#]`)
	s = colorRegex.ReplaceAllString(s, "")
	
	return s
}

// SanitizeMaterial sanitizes material names for safe storage
func SanitizeMaterial(s string) string {
	s = SanitizeString(s)
	
	// Only allow alphanumeric characters, spaces, and common material characters
	materialRegex := regexp.MustCompile(`[^a-zA-Z0-9\s\-+.]`)
	s = materialRegex.ReplaceAllString(s, "")
	
	return s
}

// SanitizeLogString sanitizes strings for safe logging
func SanitizeLogString(s string) string {
	// Remove newlines and other control chars that could break log format
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	
	// Limit length to prevent log spam
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	
	return s
}

// WriteValidationError writes validation errors as JSON response
func WriteValidationError(w http.ResponseWriter, errors ValidationErrors) {
	response.WriteValidationError(w, errors)
}

// ValidateUUID validates that a string is a valid UUID
func (v *Validator) ValidateUUID(field, value string) {
	if value == "" {
		return
	}
	
	// UUID should be exactly 36 characters: 8-4-4-4-12
	if len(value) != 36 {
		v.AddError(field, "must be a valid UUID")
		return
	}
	
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	if !uuidRegex.MatchString(value) {
		v.AddError(field, "must be a valid UUID")
	}
}

// ValidateID validates generic ID fields (used for URL parameters)
func (v *Validator) ValidateID(field, value string) {
	if value == "" {
		v.AddError(field, "ID is required")
		return
	}
	
	// Limit ID length to prevent abuse
	if len(value) > 64 {
		v.AddError(field, "ID too long (max 64 characters)")
		return
	}
	
	// For now, accept both UUIDs and simple alphanumeric IDs
	// UUID pattern
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// Simple alphanumeric ID pattern
	simpleIDRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	
	if !uuidRegex.MatchString(value) && !simpleIDRegex.MatchString(value) {
		v.AddError(field, "must be a valid ID (UUID or alphanumeric)")
	}
}

// ValidateRequestSize validates that a string doesn't exceed maximum size limits
func (v *Validator) ValidateRequestSize(field, value string, maxSize int) {
	if len(value) > maxSize {
		v.AddError(field, fmt.Sprintf("exceeds maximum size of %d characters", maxSize))
	}
}

// ValidateJSONField validates JSON field content with size limits
func (v *Validator) ValidateJSONField(field, value string, maxSize int) {
	if value == "" {
		return
	}
	
	// Check overall size
	if len(value) > maxSize {
		v.AddError(field, fmt.Sprintf("exceeds maximum size of %d characters", maxSize))
		return
	}
	
	// Check for potential JSON injection or malformed JSON
	if strings.Contains(value, "\x00") {
		v.AddError(field, "contains null bytes")
		return
	}
	
	// Count JSON nesting depth to prevent deep nesting attacks
	depth := 0
	maxDepth := 10
	for _, char := range value {
		switch char {
		case '{', '[':
			depth++
			if depth > maxDepth {
				v.AddError(field, fmt.Sprintf("JSON nesting too deep (max %d levels)", maxDepth))
				return
			}
		case '}', ']':
			depth--
		}
	}
}

// Additional length validation constants for common fields
const (
	MaxUsernameLength    = 50
	MaxEmailLength       = 254
	MaxPasswordLength    = 128
	MaxDisplayNameLength = 100
	MaxNotesLength       = 1000
	MaxColorLength       = 50
	MaxMaterialLength    = 50
	MaxFileURLLength     = 2048
	MaxIDLength          = 64
	MaxHeaderLength      = 8192
	MaxRequestBodySize   = 1024 * 1024 // 1MB
)