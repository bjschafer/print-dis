package services

import (
	"context"
	"time"

	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/models"
)

// PrintRequestService handles business logic for print requests
type PrintRequestService struct {
	db database.DBClient
}

// NewPrintRequestService creates a new print request service
func NewPrintRequestService(db database.DBClient) *PrintRequestService {
	return &PrintRequestService{
		db: db,
	}
}

// CreatePrintRequest creates a new print request
func (s *PrintRequestService) CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	// Set timestamps
	now := time.Now()
	request.CreatedAt = now
	request.UpdatedAt = now

	// Set initial status if not provided
	if request.Status == "" {
		request.Status = "pending"
	}

	return s.db.CreatePrintRequest(ctx, request)
}

// GetPrintRequest retrieves a print request by ID
func (s *PrintRequestService) GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error) {
	return s.db.GetPrintRequest(ctx, id)
}

// UpdatePrintRequest updates an existing print request
func (s *PrintRequestService) UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	// Update timestamp
	request.UpdatedAt = time.Now()

	return s.db.UpdatePrintRequest(ctx, request)
}

// DeletePrintRequest deletes a print request
func (s *PrintRequestService) DeletePrintRequest(ctx context.Context, id string) error {
	return s.db.DeletePrintRequest(ctx, id)
}

// ListPrintRequests retrieves all print requests
func (s *PrintRequestService) ListPrintRequests(ctx context.Context) ([]*models.PrintRequest, error) {
	return s.db.ListPrintRequests(ctx)
}
