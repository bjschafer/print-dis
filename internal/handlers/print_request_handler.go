package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bjschafer/print-dis/internal/middleware"
	"github.com/bjschafer/print-dis/internal/models"
	"github.com/bjschafer/print-dis/internal/response"
	"github.com/bjschafer/print-dis/internal/services"
	"github.com/bjschafer/print-dis/internal/spoolman"
	"github.com/bjschafer/print-dis/internal/validation"
	"github.com/google/uuid"
)

// PrintRequestHandler handles HTTP requests for print requests
type PrintRequestHandler struct {
	service         *services.PrintRequestService
	spoolmanService *spoolman.Service
	logger          *slog.Logger
}

// NewPrintRequestHandler creates a new print request handler
func NewPrintRequestHandler(service *services.PrintRequestService, spoolmanService *spoolman.Service) *PrintRequestHandler {
	return &PrintRequestHandler{
		service:         service,
		spoolmanService: spoolmanService,
		logger:          slog.Default(),
	}
}

// CreatePrintRequestRequest represents the request body for creating a print request
type CreatePrintRequestRequest struct {
	FileLink string  `json:"file_link"`
	Notes    string  `json:"notes"`
	SpoolID  *int    `json:"spool_id,omitempty"`
	Color    *string `json:"color,omitempty"`
	Material *string `json:"material,omitempty"`
}

// Validate validates the print request creation data
func (r *CreatePrintRequestRequest) Validate() validation.ValidationErrors {
	validator := validation.NewValidator()

	// Sanitize inputs
	r.FileLink = validation.SanitizeString(r.FileLink)
	r.Notes = validation.SanitizeNotes(r.Notes)
	if r.Color != nil {
		*r.Color = validation.SanitizeColor(*r.Color)
	}
	if r.Material != nil {
		*r.Material = validation.SanitizeMaterial(*r.Material)
	}

	// Validate
	validator.ValidateRequired("file_link", r.FileLink)
	validator.ValidateFileURL("file_link", r.FileLink)
	validator.ValidateNotes("notes", r.Notes)

	if r.Color != nil {
		validator.ValidateColor("color", *r.Color)
	}

	if r.Material != nil {
		validator.ValidateMaterial("material", *r.Material)
	}

	return validator.Errors()
}

// UpdatePrintRequestStatusRequest represents the request body for updating a print request's status
type UpdatePrintRequestStatusRequest struct {
	Status models.PrintRequestStatus `json:"status"`
}

// EnhancedPrintRequest represents a print request with additional spoolman details for admin view
type EnhancedPrintRequest struct {
	*models.PrintRequest
	SpoolDetails *spoolman.Spool `json:"spool_details,omitempty"`
}

// CreatePrintRequest handles the creation of a new print request
func (h *PrintRequestHandler) CreatePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method for create print request", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Parse request body
	var req CreatePrintRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode request body", "error", err)
		response.WriteBadRequestError(w, "Invalid request body", err.Error())
		return
	}

	// Validate input
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	// Get user ID from auth header or request body
	userID := middleware.GetUserID(r)
	if userID == "" {
		h.logger.Warn("user not authenticated")
		response.WriteUnauthorizedError(w, "Authentication required")
		return
	}

	// Create print request with sanitized data
	printRequest := &models.PrintRequest{
		ID:       uuid.New().String(),
		UserID:   userID,
		FileLink: req.FileLink,
		Notes:    req.Notes,
		SpoolID:  req.SpoolID,
		Color:    req.Color,
		Material: req.Material,
		Status:   models.StatusPendingApproval,
	}

	// Save print request
	if err := h.service.CreatePrintRequest(r.Context(), printRequest); err != nil {
		h.logger.Error("failed to create print request",
			"error", err,
			"user_id", validation.SanitizeLogString(userID),
			"file_link", validation.SanitizeLogString(req.FileLink))
		response.WriteInternalError(w, "Failed to create print request", err.Error())
		return
	}

	response.WriteCreatedResponse(w, printRequest, "Print request created successfully")
}

// GetPrintRequest handles retrieving a print request by ID
func (h *PrintRequestHandler) GetPrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for get print request", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "Print request ID is required", "")
		return
	}

	h.logger.Info("getting print request", "id", id)

	// Get print request from service
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get print request", "error", err, "id", id)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to get print request", "")
		return
	}
	if printRequest == nil {
		h.logger.Warn("print request not found", "id", id)
		response.WriteErrorResponse(w, http.StatusNotFound, response.NotFound, "Print request not found", "")
		return
	}

	response.WriteSuccessResponse(w, printRequest, "")
}

// ListPrintRequests handles retrieving all print requests
func (h *PrintRequestHandler) ListPrintRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for list print requests", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	h.logger.Info("listing all print requests")

	// Get print requests from service
	printRequests, err := h.service.ListPrintRequests(r.Context())
	if err != nil {
		h.logger.Error("failed to list print requests", "error", err)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to list print requests", "")
		return
	}

	response.WriteSuccessResponse(w, printRequests, "")
}

// ListPrintRequestsEnhanced handles retrieving all print requests with spoolman details for admin view
func (h *PrintRequestHandler) ListPrintRequestsEnhanced(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for list enhanced print requests", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Check for status filter
	status := r.URL.Query().Get("status")
	h.logger.Info("listing enhanced print requests", "status_filter", status)

	// Get print requests from service
	printRequests, err := h.service.ListPrintRequests(r.Context())
	if err != nil {
		h.logger.Error("failed to list print requests", "error", err)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to list print requests", "")
		return
	}

	// Filter by status if provided
	if status != "" {
		filteredRequests := make([]*models.PrintRequest, 0)
		for _, request := range printRequests {
			if request.Status.String() == status {
				filteredRequests = append(filteredRequests, request)
			}
		}
		printRequests = filteredRequests
	}

	// Enhance with spoolman details if spoolman is available
	enhancedRequests := make([]*EnhancedPrintRequest, len(printRequests))
	for i, request := range printRequests {
		enhanced := &EnhancedPrintRequest{
			PrintRequest: request,
		}

		// If spoolman is available and request has a spool ID, fetch spool details
		if h.spoolmanService != nil && request.SpoolID != nil {
			spoolDetails, err := h.spoolmanService.GetSpool(r.Context(), *request.SpoolID)
			if err != nil {
				h.logger.Warn("failed to fetch spool details", "spool_id", *request.SpoolID, "error", err)
				// Continue without spool details rather than failing the whole request
			} else {
				enhanced.SpoolDetails = spoolDetails
			}
		}

		enhancedRequests[i] = enhanced
	}

	response.WriteSuccessResponse(w, enhancedRequests, "")
}

// ListUserPrintRequests handles retrieving print requests for the current user
func (h *PrintRequestHandler) ListUserPrintRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for list user print requests", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Get user ID from auth middleware
	userID := middleware.GetUserID(r)
	if userID == "" {
		h.logger.Warn("user not authenticated")
		response.WriteErrorResponse(w, http.StatusUnauthorized, response.Unauthorized, "Unauthorized", "")
		return
	}

	h.logger.Info("listing print requests for user", "user_id", userID)

	// Get print requests for user directly from database
	userPrintRequests, err := h.service.ListPrintRequestsByUserID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list print requests for user", "error", err, "user_id", userID)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to list print requests", "")
		return
	}

	h.logger.Info("retrieved print requests for user",
		"user_id", userID,
		"count", len(userPrintRequests),
	)

	response.WriteSuccessResponse(w, userPrintRequests, "")
}

// UpdatePrintRequest handles updating a print request
func (h *PrintRequestHandler) UpdatePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.logger.Warn("invalid method for update print request", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		response.WriteBadRequestError(w, "Print request ID is required", "")
		return
	}

	// Validate ID format and length
	validator := validation.NewValidator()
	validator.ValidateID("id", id)
	if validationErrors := validator.Errors(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	h.logger.Info("updating print request", "id", validation.SanitizeLogString(id))

	// Get existing print request
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get print request for update", "error", err, "id", validation.SanitizeLogString(id))
		response.WriteInternalError(w, "Failed to get print request", err.Error())
		return
	}
	if printRequest == nil {
		h.logger.Warn("print request not found for update", "id", validation.SanitizeLogString(id))
		response.WriteNotFoundError(w, "Print request not found")
		return
	}

	// Decode request body
	var req CreatePrintRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode update request body", "error", err, "id", validation.SanitizeLogString(id))
		response.WriteBadRequestError(w, "Invalid request body", err.Error())
		return
	}

	// Validate input
	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	// Get user ID from auth header or request body
	userID := middleware.GetUserID(r)
	if userID == "" {
		h.logger.Warn("user not authenticated")
		response.WriteUnauthorizedError(w, "Authentication required")
		return
	}

	// Update print request fields with sanitized data
	printRequest.UserID = userID
	printRequest.FileLink = req.FileLink
	printRequest.Notes = req.Notes
	printRequest.SpoolID = req.SpoolID
	printRequest.Color = req.Color
	printRequest.Material = req.Material

	h.logger.Info("updating print request fields",
		"id", validation.SanitizeLogString(printRequest.ID),
		"user_id", validation.SanitizeLogString(printRequest.UserID),
		"file_link", validation.SanitizeLogString(printRequest.FileLink),
		"spool_id", printRequest.SpoolID,
	)

	// Save updates through service layer
	if err := h.service.UpdatePrintRequest(r.Context(), printRequest); err != nil {
		h.logger.Error("failed to update print request", "error", err, "id", validation.SanitizeLogString(id))
		response.WriteInternalError(w, "Failed to update print request", err.Error())
		return
	}

	response.WriteSuccessResponse(w, printRequest, "Print request updated successfully")
}

// DeletePrintRequest handles deleting a print request
func (h *PrintRequestHandler) DeletePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.logger.Warn("invalid method for delete print request", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		response.WriteErrorResponse(w, http.StatusBadRequest, response.BadRequest, "Print request ID is required", "")
		return
	}

	h.logger.Info("deleting print request", "id", id)

	// Delete print request through service layer
	if err := h.service.DeletePrintRequest(r.Context(), id); err != nil {
		h.logger.Error("failed to delete print request", "error", err, "id", id)
		response.WriteErrorResponse(w, http.StatusInternalServerError, response.InternalError, "Failed to delete print request", "")
		return
	}

	response.WriteSuccessResponse(w, nil, "Print request deleted successfully")
}

// UpdatePrintRequestStatus handles updating a print request's status
func (h *PrintRequestHandler) UpdatePrintRequestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.logger.Warn("invalid method for update print request status", "method", r.Method)
		response.WriteErrorResponse(w, http.StatusMethodNotAllowed, response.BadRequest, "Method not allowed", "")
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		response.WriteBadRequestError(w, "Print request ID is required", "")
		return
	}

	// Validate ID format and length
	validator := validation.NewValidator()
	validator.ValidateID("id", id)
	if validationErrors := validator.Errors(); len(validationErrors) > 0 {
		validation.WriteValidationError(w, validationErrors)
		return
	}

	h.logger.Info("updating print request status", "id", validation.SanitizeLogString(id))

	// Get existing print request
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get print request for status update", "error", err, "id", validation.SanitizeLogString(id))
		response.WriteInternalError(w, "Failed to get print request", err.Error())
		return
	}
	if printRequest == nil {
		h.logger.Warn("print request not found for status update", "id", validation.SanitizeLogString(id))
		response.WriteNotFoundError(w, "Print request not found")
		return
	}

	// Decode request body
	var req UpdatePrintRequestStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode status update request body", "error", err, "id", validation.SanitizeLogString(id))
		response.WriteBadRequestError(w, "Invalid request body", err.Error())
		return
	}

	// Validate status
	if !req.Status.IsAPrintRequestStatus() {
		h.logger.Warn("invalid status value", "status", req.Status)
		response.WriteBadRequestError(w, "Invalid status value", "")
		return
	}

	// Update status
	printRequest.Status = req.Status

	h.logger.Info("updating print request status",
		"id", validation.SanitizeLogString(printRequest.ID),
		"status", printRequest.Status.String(),
	)

	// Save updates through service layer
	if err := h.service.UpdatePrintRequest(r.Context(), printRequest); err != nil {
		h.logger.Error("failed to update print request status", "error", err, "id", validation.SanitizeLogString(id))
		response.WriteInternalError(w, "Failed to update print request status", err.Error())
		return
	}

	response.WriteSuccessResponse(w, printRequest, "Print request status updated successfully")
}
