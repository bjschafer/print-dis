package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal name",
			input:    "John Doe",
			expected: "John Doe",
		},
		{
			name:     "name with HTML tags",
			input:    "John <script>alert('xss')</script> Doe",
			expected: "John alert(xss) Doe",
		},
		{
			name:     "name with dangerous characters",
			input:    `John "Hacker" <>&' Doe`,
			expected: "John Hacker  Doe",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeDisplayName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeNotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal notes",
			input:    "This is a normal note",
			expected: "This is a normal note",
		},
		{
			name:     "notes with HTML",
			input:    "This is <b>bold</b> text",
			expected: "This is bold text",
		},
		{
			name:     "notes with JavaScript",
			input:    "Click here: javascript:alert('xss')",
			expected: "Click here: alert('xss')",
		},
		{
			name:     "notes with script tag",
			input:    "Normal text <script>alert('xss')</script> more text",
			expected: "Normal text alert('xss') more text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeNotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeColor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal color",
			input:    "red",
			expected: "red",
		},
		{
			name:     "hex color",
			input:    "#ff0000",
			expected: "#ff0000",
		},
		{
			name:     "color with spaces",
			input:    "light blue",
			expected: "light blue",
		},
		{
			name:     "color with invalid characters",
			input:    "red<script>alert('xss')</script>",
			expected: "redscriptalertxssscript",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeColor(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateFileURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid HTTP URL",
			input:       "http://example.com/file.stl",
			shouldError: false,
		},
		{
			name:        "valid HTTPS URL",
			input:       "https://example.com/file.stl",
			shouldError: false,
		},
		{
			name:        "invalid scheme",
			input:       "ftp://example.com/file.stl",
			shouldError: true,
			errorMsg:    "must use HTTP or HTTPS",
		},
		{
			name:        "JavaScript URL",
			input:       "javascript:alert('xss')",
			shouldError: true,
			errorMsg:    "cannot contain JavaScript URLs",
		},
		{
			name:        "data URL",
			input:       "data:text/html,<script>alert('xss')</script>",
			shouldError: true,
			errorMsg:    "data URLs are not allowed",
		},
		{
			name:        "invalid URL",
			input:       "not-a-url",
			shouldError: true,
			errorMsg:    "must be a valid URL",
		},
		{
			name:        "empty URL",
			input:       "",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			validator.ValidateFileURL("file_link", tt.input)
			errors := validator.Errors()

			if tt.shouldError {
				assert.NotEmpty(t, errors)
				if tt.errorMsg != "" {
					found := false
					for _, err := range errors {
						if err.Field == "file_link" && err.Message == tt.errorMsg {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected error message '%s' not found", tt.errorMsg)
				}
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:        "valid display name",
			input:       "John Doe",
			shouldError: false,
		},
		{
			name:        "display name with HTML characters",
			input:       "John <script> Doe",
			shouldError: true,
		},
		{
			name:        "display name with HTML tags",
			input:       "John <b>Bold</b> Doe",
			shouldError: true,
		},
		{
			name:        "very long display name",
			input:       string(make([]byte, 101)), // 101 characters
			shouldError: true,
		},
		{
			name:        "empty display name",
			input:       "",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			validator.ValidateDisplayName("display_name", tt.input)
			errors := validator.Errors()

			if tt.shouldError {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func TestSanitizeLogString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal string",
			input:    "normal log message",
			expected: "normal log message",
		},
		{
			name:     "string with newlines",
			input:    "line1\nline2\rline3",
			expected: "line1\\nline2\\rline3",
		},
		{
			name:     "string with tabs",
			input:    "tab\tseparated\tvalues",
			expected: "tab\\tseparated\\tvalues",
		},
		{
			name:     "very long string",
			input:    string(make([]byte, 250)),
			expected: string(make([]byte, 200)) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLogString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:        "valid UUID",
			input:       "123e4567-e89b-12d3-a456-426614174000",
			shouldError: false,
		},
		{
			name:        "empty UUID",
			input:       "",
			shouldError: false,
		},
		{
			name:        "invalid UUID format",
			input:       "not-a-uuid",
			shouldError: true,
		},
		{
			name:        "UUID too short",
			input:       "123e4567-e89b-12d3-a456-42661417400",
			shouldError: true,
		},
		{
			name:        "UUID too long",
			input:       "123e4567-e89b-12d3-a456-4266141740000",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			validator.ValidateUUID("uuid", tt.input)
			errors := validator.Errors()

			if tt.shouldError {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func TestValidateID(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:        "valid UUID",
			input:       "123e4567-e89b-12d3-a456-426614174000",
			shouldError: false,
		},
		{
			name:        "valid alphanumeric ID",
			input:       "abc123_-",
			shouldError: false,
		},
		{
			name:        "empty ID",
			input:       "",
			shouldError: true,
		},
		{
			name:        "ID too long",
			input:       "a" + string(make([]byte, 64)), // 65 characters
			shouldError: true,
		},
		{
			name:        "invalid characters",
			input:       "invalid@id!",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			validator.ValidateID("id", tt.input)
			errors := validator.Errors()

			if tt.shouldError {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func TestValidateRequestSize(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		maxSize     int
		shouldError bool
	}{
		{
			name:        "valid size",
			input:       "hello",
			maxSize:     10,
			shouldError: false,
		},
		{
			name:        "at max size",
			input:       "hello",
			maxSize:     5,
			shouldError: false,
		},
		{
			name:        "exceeds max size",
			input:       "hello world",
			maxSize:     5,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			validator.ValidateRequestSize("field", tt.input, tt.maxSize)
			errors := validator.Errors()

			if tt.shouldError {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func TestValidateJSONField(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		maxSize     int
		shouldError bool
	}{
		{
			name:        "valid JSON field",
			input:       `{"key": "value"}`,
			maxSize:     100,
			shouldError: false,
		},
		{
			name:        "field too large",
			input:       string(make([]byte, 101)),
			maxSize:     100,
			shouldError: true,
		},
		{
			name:        "contains null bytes",
			input:       "test\x00string",
			maxSize:     100,
			shouldError: true,
		},
		{
			name:        "JSON too deeply nested",
			input:       `{"a":{"b":{"c":{"d":{"e":{"f":{"g":{"h":{"i":{"j":{"k":"deep"}}}}}}}}}}`,
			maxSize:     200,
			shouldError: true,
		},
		{
			name:        "empty field",
			input:       "",
			maxSize:     100,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			validator.ValidateJSONField("json_field", tt.input, tt.maxSize)
			errors := validator.Errors()

			if tt.shouldError {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}