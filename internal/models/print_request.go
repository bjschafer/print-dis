//go:generate -command enumer go tool enumer
package models

import "time"

// PrintRequestStatus represents the possible states of a print request
//
//go:generate enumer -type=PrintRequestStatus -json -text -output=print_request_status_gen.go
type PrintRequestStatus int

const (
	StatusPendingApproval PrintRequestStatus = iota
	StatusEnqueued
	StatusInProgress
	StatusDone
)

// PrintRequest represents a 3D printing request from a user
type PrintRequest struct {
	ID        string             `json:"id" db:"id"`
	UserID    string             `json:"user_id" db:"user_id"`
	FileLink  string             `json:"file_link" db:"file_link"`
	Notes     string             `json:"notes" db:"notes"`
	SpoolID   *string            `json:"spool_id,omitempty" db:"spool_id"` // Optional Spoolman spool ID
	Color     *string            `json:"color,omitempty" db:"color"`       // Optional color preference
	Material  *string            `json:"material,omitempty" db:"material"` // Optional material preference
	Status    PrintRequestStatus `json:"status" db:"status"`
	CreatedAt time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt time.Time          `json:"updated_at" db:"updated_at"`
}

// NewPrintRequest creates a new print request with default values
func NewPrintRequest(userID, fileLink, notes string) *PrintRequest {
	now := time.Now()
	return &PrintRequest{
		UserID:    userID,
		FileLink:  fileLink,
		Notes:     notes,
		Status:    StatusPendingApproval,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
