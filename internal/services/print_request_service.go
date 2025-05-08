package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/models"
)

// PrintRequestService handles business logic for print requests
type PrintRequestService struct {
	db     database.DBClient
	logger *slog.Logger
}

// NewPrintRequestService creates a new print request service
func NewPrintRequestService(db database.DBClient) *PrintRequestService {
	return &PrintRequestService{
		db:     db,
		logger: slog.Default(),
	}
}

// CreatePrintRequest creates a new print request
func (s *PrintRequestService) CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	// Set timestamps
	now := time.Now()
	request.CreatedAt = now
	request.UpdatedAt = now

	// Set initial status if not provided
	if request.Status == 0 {
		request.Status = models.StatusPendingApproval
	}

	s.logger.Info("creating print request in database",
		"id", request.ID,
		"user_id", request.UserID,
		"status", request.Status.String(),
	)

	if err := s.db.CreatePrintRequest(ctx, request); err != nil {
		s.logger.Error("failed to create print request in database",
			"error", err,
			"id", request.ID,
		)
		return err
	}

	return nil
}

// GetPrintRequest retrieves a print request by ID
func (s *PrintRequestService) GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error) {
	s.logger.Info("retrieving print request from database", "id", id)

	request, err := s.db.GetPrintRequest(ctx, id)
	if err != nil {
		s.logger.Error("failed to retrieve print request from database",
			"error", err,
			"id", id,
		)
		return nil, err
	}

	if request == nil {
		s.logger.Warn("print request not found in database", "id", id)
	}

	return request, nil
}

// validateStatusTransition checks if the status transition is valid
func (s *PrintRequestService) validateStatusTransition(currentStatus, newStatus models.PrintRequestStatus) error {
	// Define valid transitions
	validTransitions := map[models.PrintRequestStatus][]models.PrintRequestStatus{
		models.StatusPendingApproval: {models.StatusEnqueued, models.StatusInProgress},
		models.StatusEnqueued:        {models.StatusInProgress},
		models.StatusInProgress:      {models.StatusDone},
		models.StatusDone:            {}, // No valid transitions from Done
	}

	// Check if the transition is valid
	allowedTransitions := validTransitions[currentStatus]
	for _, allowedStatus := range allowedTransitions {
		if newStatus == allowedStatus {
			return nil
		}
	}

	return fmt.Errorf("invalid status transition from %s to %s", currentStatus.String(), newStatus.String())
}

// UpdatePrintRequest updates an existing print request
func (s *PrintRequestService) UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	// Get current state
	currentRequest, err := s.GetPrintRequest(ctx, request.ID)
	if err != nil {
		return fmt.Errorf("failed to get current print request: %w", err)
	}
	if currentRequest == nil {
		return fmt.Errorf("print request not found: %s", request.ID)
	}

	// Validate status transition if status is being changed
	if currentRequest.Status != request.Status {
		if err := s.validateStatusTransition(currentRequest.Status, request.Status); err != nil {
			s.logger.Warn("invalid status transition",
				"id", request.ID,
				"current_status", currentRequest.Status.String(),
				"new_status", request.Status.String(),
				"error", err,
			)
			return fmt.Errorf("invalid status update: %w", err)
		}
	}

	// Update timestamp
	request.UpdatedAt = time.Now()

	s.logger.Info("updating print request in database",
		"id", request.ID,
		"user_id", request.UserID,
		"status", request.Status.String(),
		"previous_status", currentRequest.Status.String(),
	)

	if err := s.db.UpdatePrintRequest(ctx, request); err != nil {
		s.logger.Error("failed to update print request in database",
			"error", err,
			"id", request.ID,
		)
		return err
	}

	return nil
}

// DeletePrintRequest deletes a print request
func (s *PrintRequestService) DeletePrintRequest(ctx context.Context, id string) error {
	s.logger.Info("deleting print request from database", "id", id)

	if err := s.db.DeletePrintRequest(ctx, id); err != nil {
		s.logger.Error("failed to delete print request from database",
			"error", err,
			"id", id,
		)
		return err
	}

	return nil
}

// ListPrintRequests retrieves all print requests
func (s *PrintRequestService) ListPrintRequests(ctx context.Context) ([]*models.PrintRequest, error) {
	s.logger.Info("retrieving all print requests from database")

	requests, err := s.db.ListPrintRequests(ctx)
	if err != nil {
		s.logger.Error("failed to retrieve print requests from database",
			"error", err,
		)
		return nil, err
	}

	s.logger.Info("retrieved print requests from database",
		"count", len(requests),
	)

	return requests, nil
}
