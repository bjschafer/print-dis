package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/bjschafer/print-dis/internal/services"
	"github.com/google/uuid"
)

// PrintRequestHandler handles HTTP requests for print requests
type PrintRequestHandler struct {
	service *services.PrintRequestService
}

// NewPrintRequestHandler creates a new print request handler
func NewPrintRequestHandler(service *services.PrintRequestService) *PrintRequestHandler {
	return &PrintRequestHandler{
		service: service,
	}
}

// CreatePrintRequestRequest represents the request body for creating a print request
type CreatePrintRequestRequest struct {
	UserID   string  `json:"user_id"`
	FileLink string  `json:"file_link"`
	Notes    string  `json:"notes"`
	SpoolID  *string `json:"spool_id,omitempty"`
	Color    *string `json:"color,omitempty"`
	Material *string `json:"material,omitempty"`
}

// CreatePrintRequest handles the creation of a new print request
func (h *PrintRequestHandler) CreatePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreatePrintRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.UserID == "" || req.FileLink == "" {
		http.Error(w, "User ID and file link are required", http.StatusBadRequest)
		return
	}

	// Create new print request
	printRequest := models.NewPrintRequest(req.UserID, req.FileLink, req.Notes)
	printRequest.ID = uuid.New().String()
	printRequest.SpoolID = req.SpoolID
	printRequest.Color = req.Color
	printRequest.Material = req.Material

	// Save to database through service layer
	if err := h.service.CreatePrintRequest(r.Context(), printRequest); err != nil {
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	// Get print request from service
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to get print request", http.StatusInternalServerError)
		return
	}
	if printRequest == nil {
		http.Error(w, "Print request not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequest)
}

// ListPrintRequests handles retrieving all print requests
func (h *PrintRequestHandler) ListPrintRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get print requests from service
	printRequests, err := h.service.ListPrintRequests(r.Context())
	if err != nil {
		http.Error(w, "Failed to list print requests", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequests)
}

// UpdatePrintRequest handles updating a print request
func (h *PrintRequestHandler) UpdatePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	// Get existing print request
	printRequest, err := h.service.GetPrintRequest(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to get print request", http.StatusInternalServerError)
		return
	}
	if printRequest == nil {
		http.Error(w, "Print request not found", http.StatusNotFound)
		return
	}

	// Decode request body
	var req CreatePrintRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	// Save updates through service layer
	if err := h.service.UpdatePrintRequest(r.Context(), printRequest); err != nil {
		http.Error(w, "Failed to update print request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printRequest)
}

// DeletePrintRequest handles deleting a print request
func (h *PrintRequestHandler) DeletePrintRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Print request ID is required", http.StatusBadRequest)
		return
	}

	// Delete print request through service layer
	if err := h.service.DeletePrintRequest(r.Context(), id); err != nil {
		http.Error(w, "Failed to delete print request", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
