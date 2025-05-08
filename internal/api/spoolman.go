package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/bjschafer/print-dis/internal/spoolman"
)

type SpoolmanHandler struct {
	service *spoolman.Service
}

func NewSpoolmanHandler(service *spoolman.Service) *SpoolmanHandler {
	return &SpoolmanHandler{
		service: service,
	}
}

// GetSpools returns all available spools from Spoolman
func (h *SpoolmanHandler) GetSpools(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handling get spools request")

	spools, err := h.service.GetSpools(r.Context())
	if err != nil {
		slog.Error("failed to get spools", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Debug("successfully retrieved spools", "count", len(spools))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(spools); err != nil {
		slog.Error("failed to encode spools response", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetSpool returns a specific spool by ID
func (h *SpoolmanHandler) GetSpool(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		slog.Warn("get spool request missing id parameter")
		http.Error(w, "spool ID is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		slog.Warn("invalid spool ID", "id", idStr, "error", err)
		http.Error(w, "invalid spool ID", http.StatusBadRequest)
		return
	}

	slog.Debug("handling get spool request", "id", id)
	spool, err := h.service.GetSpool(r.Context(), id)
	if err != nil {
		slog.Error("failed to get spool", "id", id, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Debug("successfully retrieved spool", "id", id)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(spool); err != nil {
		slog.Error("failed to encode spool response", "id", id, "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetMaterials returns all unique materials from Spoolman
func (h *SpoolmanHandler) GetMaterials(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handling get materials request")

	materials, err := h.service.GetMaterials(r.Context())
	if err != nil {
		slog.Error("failed to get materials", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Debug("successfully retrieved materials", "count", len(materials))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(materials); err != nil {
		slog.Error("failed to encode materials response", "error", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
