package validation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
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

// WriteValidationError writes validation errors as JSON response
func WriteValidationError(w http.ResponseWriter, errors ValidationErrors) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	
	response := map[string]interface{}{
		"error":   "validation_failed",
		"message": "Input validation failed",
		"details": errors,
	}
	
	json.NewEncoder(w).Encode(response)
}