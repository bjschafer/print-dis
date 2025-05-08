package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/bjschafer/print-dis/internal/services"
	"github.com/google/uuid"
)

// PrintRequestHandler handles HTTP requests for print requests
type PrintRequestHandler struct {
	service *services.PrintRequestService
	logger  *slog.Logger
}

// NewPrintRequestHandler creates a new print request handler
func NewPrintRequestHandler(service *services.PrintRequestService) *PrintRequestHandler {
	return &PrintRequestHandler{
		service: service,
		logger:  slog.Default(),
	}
}

// CreatePrintRequestRequest represents the request body for creating a print request
type CreatePrintRequestRequest struct {
	UserID   string  `json:"user_id"`
	FileLink string  `json:"file_link"`
	Notes    string  `json:"notes"`
	SpoolID  *int    `json:"spool_id,omitempty"`
	Color    *string `json:"color,omitempty"`
	Material *string `json:"material,omitempty"`
}

// UpdatePrintRequestStatusRequest represents the request body for updating a print request's status
type UpdatePrintRequestStatusRequest struct {
	Status models.PrintRequestStatus `json:"status"`
}

// CreatePrintRequest handles the creation of a new print request
func (h *PrintRequestHandler) CreatePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method for create print request", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreatePrintRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.UserID == "" || req.FileLink == "" {
		h.logger.Warn("missing required fields", "user_id", req.UserID, "file_link", req.FileLink)
		http.Error(w, "User ID and file link are required", http.StatusBadRequest)
		return
	}

	// Create new print request
	printRequest := models.NewPrintRequest(req.UserID, req.FileLink, req.Notes)
	printRequest.ID = uuid.New().String()
	printRequest.SpoolID = req.SpoolID
	printRequest.Color = req.Color
	printRequest.Material = req.Material

	h.logger.Info("creating new print request",
		"id", printRequest.ID,
		"user_id", printRequest.UserID,
		"file_link", printRequest.FileLink,
		"spool_id", printRequest.SpoolID,
		"color", printRequest.Color,
		"material", printRequest.Material,
	)

	// Save to database through service layer
	if err := h.service.CreatePrintRequest(r.Context(), printRequest); err != nil {
		h.logger.Error("failed to create print request", "error", err, "id", printRequest.ID)
		http.Error(w, "Failed to create print request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(printRequest)
}

// GetPrintRequest handles retrieving a print request by ID
func (h *PrintRequestHandler) GetPrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for get print request", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	h.logger.Info("getting print request", "id", id)

	// Get print request from service
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get print request", "error", err, "id", id)
		http.Error(w, "Failed to get print request", http.StatusInternalServerError)
		return
	}
	if printRequest == nil {
		h.logger.Warn("print request not found", "id", id)
		http.Error(w, "Print request not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequest)
}

// ListPrintRequests handles retrieving all print requests
func (h *PrintRequestHandler) ListPrintRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for list print requests", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.logger.Info("listing all print requests")

	// Get print requests from service
	printRequests, err := h.service.ListPrintRequests(r.Context())
	if err != nil {
		h.logger.Error("failed to list print requests", "error", err)
		http.Error(w, "Failed to list print requests", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequests)
}

// UpdatePrintRequest handles updating a print request
func (h *PrintRequestHandler) UpdatePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.logger.Warn("invalid method for update print request", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	h.logger.Info("updating print request", "id", id)

	// Get existing print request
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get print request for update", "error", err, "id", id)
		http.Error(w, "Failed to get print request", http.StatusInternalServerError)
		return
	}
	if printRequest == nil {
		h.logger.Warn("print request not found for update", "id", id)
		http.Error(w, "Print request not found", http.StatusNotFound)
		return
	}

	// Decode request body
	var req CreatePrintRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode update request body", "error", err, "id", id)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update print request fields
	printRequest.UserID = req.UserID
	printRequest.FileLink = req.FileLink
	printRequest.Notes = req.Notes
	printRequest.SpoolID = req.SpoolID
	printRequest.Color = req.Color
	printRequest.Material = req.Material

	h.logger.Info("updating print request fields",
		"id", printRequest.ID,
		"user_id", printRequest.UserID,
		"file_link", printRequest.FileLink,
		"spool_id", printRequest.SpoolID,
		"color", printRequest.Color,
		"material", printRequest.Material,
	)

	// Save updates through service layer
	if err := h.service.UpdatePrintRequest(r.Context(), printRequest); err != nil {
		h.logger.Error("failed to update print request", "error", err, "id", id)
		http.Error(w, "Failed to update print request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequest)
}

// DeletePrintRequest handles deleting a print request
func (h *PrintRequestHandler) DeletePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.logger.Warn("invalid method for delete print request", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	h.logger.Info("deleting print request", "id", id)

	// Delete print request through service layer
	if err := h.service.DeletePrintRequest(r.Context(), id); err != nil {
		h.logger.Error("failed to delete print request", "error", err, "id", id)
		http.Error(w, "Failed to delete print request", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdatePrintRequestStatus handles updating a print request's status
func (h *PrintRequestHandler) UpdatePrintRequestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.logger.Warn("invalid method for update print request status", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		h.logger.Warn("missing print request ID")
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	h.logger.Info("updating print request status", "id", id)

	// Get existing print request
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get print request for status update", "error", err, "id", id)
		http.Error(w, "Failed to get print request", http.StatusInternalServerError)
		return
	}
	if printRequest == nil {
		h.logger.Warn("print request not found for status update", "id", id)
		http.Error(w, "Print request not found", http.StatusNotFound)
		return
	}

	// Decode request body
	var req UpdatePrintRequestStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode status update request body", "error", err, "id", id)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate status
	if !req.Status.IsAPrintRequestStatus() {
		h.logger.Warn("invalid status value", "status", req.Status)
		http.Error(w, "Invalid status value", http.StatusBadRequest)
		return
	}

	// Update status
	printRequest.Status = req.Status

	h.logger.Info("updating print request status",
		"id", printRequest.ID,
		"status", printRequest.Status.String(),
	)

	// Save updates through service layer
	if err := h.service.UpdatePrintRequest(r.Context(), printRequest); err != nil {
		h.logger.Error("failed to update print request status", "error", err, "id", id)
		http.Error(w, "Failed to update print request status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequest)
}
