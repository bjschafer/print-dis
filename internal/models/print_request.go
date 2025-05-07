package models

import "time"

// PrintRequest represents a 3D printing request from a user
type PrintRequest struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	FileLink  string    `json:"file_link"`
	Notes     string    `json:"notes"`
	SpoolID   *string   `json:"spool_id,omitempty"` // Optional Spoolman spool ID
	Color     *string   `json:"color,omitempty"`    // Optional color preference
	Material  *string   `json:"material,omitempty"` // Optional material preference
	Status    string    `json:"status"`             // e.g., "pending", "approved", "printing", "completed"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewPrintRequest creates a new print request with default values
func NewPrintRequest(userID, fileLink, notes string) *PrintRequest {
	now := time.Now()
	return &PrintRequest{
		UserID:    userID,
		FileLink:  fileLink,
		Notes:     notes,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
}
