package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorCode represents standardized error codes
type ErrorCode string

const (
	ValidationFailed ErrorCode = "VALIDATION_FAILED"
	Unauthorized     ErrorCode = "UNAUTHORIZED"
	Forbidden        ErrorCode = "FORBIDDEN"
	NotFound         ErrorCode = "NOT_FOUND"
	Conflict         ErrorCode = "CONFLICT"
	InternalError    ErrorCode = "INTERNAL_ERROR"
	BadRequest       ErrorCode = "BAD_REQUEST"
)

// SuccessResponse represents a successful API response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// ErrorResponse represents an error API response
type ErrorResponse struct {
	Success bool      `json:"success"`
	Error   ErrorInfo `json:"error"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
}

// WriteJSONResponse writes a JSON response with the specified status code
func WriteJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// WriteSuccessResponse writes a successful JSON response
func WriteSuccessResponse(w http.ResponseWriter, data interface{}, message string) {
	response := SuccessResponse{
		Success: true,
		Data:    data,
		Message: message,
	}
	WriteJSONResponse(w, http.StatusOK, response)
}

// WriteCreatedResponse writes a successful creation response
func WriteCreatedResponse(w http.ResponseWriter, data interface{}, message string) {
	response := SuccessResponse{
		Success: true,
		Data:    data,
		Message: message,
	}
	WriteJSONResponse(w, http.StatusCreated, response)
}

// WriteNoContentResponse writes a successful no-content response
func WriteNoContentResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// WriteErrorResponse writes an error JSON response
func WriteErrorResponse(w http.ResponseWriter, statusCode int, errorCode ErrorCode, message string, details string) {
	response := ErrorResponse{
		Success: false,
		Error: ErrorInfo{
			Code:    errorCode,
			Message: message,
			Details: details,
		},
	}
	WriteJSONResponse(w, statusCode, response)
}

// WriteBadRequestError writes a 400 Bad Request error response
func WriteBadRequestError(w http.ResponseWriter, message string, details string) {
	WriteErrorResponse(w, http.StatusBadRequest, BadRequest, message, details)
}

// WriteUnauthorizedError writes a 401 Unauthorized error response
func WriteUnauthorizedError(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, http.StatusUnauthorized, Unauthorized, message, "")
}

// WriteForbiddenError writes a 403 Forbidden error response
func WriteForbiddenError(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, http.StatusForbidden, Forbidden, message, "")
}

// WriteNotFoundError writes a 404 Not Found error response
func WriteNotFoundError(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, http.StatusNotFound, NotFound, message, "")
}

// WriteConflictError writes a 409 Conflict error response
func WriteConflictError(w http.ResponseWriter, message string, details string) {
	WriteErrorResponse(w, http.StatusConflict, Conflict, message, details)
}

// WriteInternalError writes a 500 Internal Server Error response
func WriteInternalError(w http.ResponseWriter, message string, details string) {
	WriteErrorResponse(w, http.StatusInternalServerError, InternalError, message, details)
}

// WriteValidationError writes a validation error response
func WriteValidationError(w http.ResponseWriter, validationErrors interface{}) {
	response := ErrorResponse{
		Success: false,
		Error: ErrorInfo{
			Code:    ValidationFailed,
			Message: "Input validation failed",
			Details: "",
		},
	}
	
	// If we have validation errors, include them in the response
	if validationErrors != nil {
		// Create a custom response that includes the validation details
		customResponse := map[string]interface{}{
			"success": false,
			"error": map[string]interface{}{
				"code":    ValidationFailed,
				"message": "Input validation failed",
				"details": validationErrors,
			},
		}
		WriteJSONResponse(w, http.StatusBadRequest, customResponse)
		return
	}
	
	WriteJSONResponse(w, http.StatusBadRequest, response)
}