package services

import (
	"context"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDBClient is a mock implementation of database.DBClient
type MockDBClient struct {
	mock.Mock
}

func (m *MockDBClient) CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockDBClient) GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PrintRequest), args.Error(1)
}

func (m *MockDBClient) UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockDBClient) DeletePrintRequest(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDBClient) ListPrintRequests(ctx context.Context) ([]*models.PrintRequest, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*models.PrintRequest), args.Error(1)
}

// Implement other required interface methods...
func (m *MockDBClient) CreatePrinter(ctx context.Context, printer *models.Printer) error {
	return nil
}
func (m *MockDBClient) GetPrinter(ctx context.Context, id int) (*models.Printer, error) {
	return nil, nil
}
func (m *MockDBClient) UpdatePrinter(ctx context.Context, printer *models.Printer) error {
	return nil
}
func (m *MockDBClient) DeletePrinter(ctx context.Context, id int) error {
	return nil
}
func (m *MockDBClient) ListPrinters(ctx context.Context) ([]*models.Printer, error) {
	return nil, nil
}
func (m *MockDBClient) CreateFilament(ctx context.Context, filament *models.Filament) error {
	return nil
}
func (m *MockDBClient) GetFilament(ctx context.Context, id int) (*models.Filament, error) {
	return nil, nil
}
func (m *MockDBClient) UpdateFilament(ctx context.Context, filament *models.Filament) error {
	return nil
}
func (m *MockDBClient) DeleteFilament(ctx context.Context, id int) error {
	return nil
}
func (m *MockDBClient) ListFilaments(ctx context.Context) ([]*models.Filament, error) {
	return nil, nil
}
func (m *MockDBClient) CreateJob(ctx context.Context, job *models.Job) error {
	return nil
}
func (m *MockDBClient) GetJob(ctx context.Context, id int) (*models.Job, error) {
	return nil, nil
}
func (m *MockDBClient) UpdateJob(ctx context.Context, job *models.Job) error {
	return nil
}
func (m *MockDBClient) DeleteJob(ctx context.Context, id int) error {
	return nil
}
func (m *MockDBClient) ListJobs(ctx context.Context) ([]*models.Job, error) {
	return nil, nil
}
func (m *MockDBClient) CreateMaterial(ctx context.Context, material *models.Material) error {
	return nil
}
func (m *MockDBClient) GetMaterial(ctx context.Context, id int) (*models.Material, error) {
	return nil, nil
}
func (m *MockDBClient) UpdateMaterial(ctx context.Context, material *models.Material) error {
	return nil
}
func (m *MockDBClient) DeleteMaterial(ctx context.Context, id int) error {
	return nil
}
func (m *MockDBClient) ListMaterials(ctx context.Context) ([]*models.Material, error) {
	return nil, nil
}

// Close implements the database.DBClient interface
func (m *MockDBClient) Close() error {
	return nil
}

func TestPrintRequestStatusValidation(t *testing.T) {
	mockDB := new(MockDBClient)
	service := NewPrintRequestService(mockDB)
	ctx := context.Background()

	// Create a test print request
	request := &models.PrintRequest{
		ID:        uuid.New().String(),
		UserID:    "test-user",
		FileLink:  "test-file",
		Status:    models.StatusPendingApproval,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	tests := []struct {
		name          string
		currentStatus models.PrintRequestStatus
		newStatus     models.PrintRequestStatus
		shouldError   bool
	}{
		{
			name:          "Valid transition: PendingApproval to Enqueued",
			currentStatus: models.StatusPendingApproval,
			newStatus:     models.StatusEnqueued,
			shouldError:   false,
		},
		{
			name:          "Valid transition: PendingApproval to InProgress",
			currentStatus: models.StatusPendingApproval,
			newStatus:     models.StatusInProgress,
			shouldError:   false,
		},
		{
			name:          "Valid transition: Enqueued to InProgress",
			currentStatus: models.StatusEnqueued,
			newStatus:     models.StatusInProgress,
			shouldError:   false,
		},
		{
			name:          "Valid transition: InProgress to Done",
			currentStatus: models.StatusInProgress,
			newStatus:     models.StatusDone,
			shouldError:   false,
		},
		{
			name:          "Invalid transition: PendingApproval to Done",
			currentStatus: models.StatusPendingApproval,
			newStatus:     models.StatusDone,
			shouldError:   true,
		},
		{
			name:          "Invalid transition: Enqueued to Done",
			currentStatus: models.StatusEnqueued,
			newStatus:     models.StatusDone,
			shouldError:   true,
		},
		{
			name:          "Invalid transition: Done to InProgress",
			currentStatus: models.StatusDone,
			newStatus:     models.StatusInProgress,
			shouldError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock expectations
			mockDB.ExpectedCalls = nil

			// Create a copy of the request for this test
			testRequest := *request
			testRequest.Status = tt.currentStatus

			// Set up the mock for GetPrintRequest
			mockDB.On("GetPrintRequest", ctx, testRequest.ID).Return(&testRequest, nil)

			// Only set up UpdatePrintRequest mock for valid transitions
			if !tt.shouldError {
				mockDB.On("UpdatePrintRequest", ctx, mock.MatchedBy(func(req *models.PrintRequest) bool {
					return req.ID == testRequest.ID && req.Status == tt.newStatus
				})).Return(nil)
			}

			// Try to update the status
			updatedRequest := *request
			updatedRequest.Status = tt.newStatus
			err := service.UpdatePrintRequest(ctx, &updatedRequest)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid status transition")
			} else {
				assert.NoError(t, err)
			}

			// Verify all expectations were met
			mockDB.AssertExpectations(t)
		})
	}
}

func TestPrintRequestStatusEnum(t *testing.T) {
	tests := []struct {
		name     string
		status   models.PrintRequestStatus
		expected string
	}{
		{
			name:     "PendingApproval",
			status:   models.StatusPendingApproval,
			expected: "StatusPendingApproval",
		},
		{
			name:     "Enqueued",
			status:   models.StatusEnqueued,
			expected: "StatusEnqueued",
		},
		{
			name:     "InProgress",
			status:   models.StatusInProgress,
			expected: "StatusInProgress",
		},
		{
			name:     "Done",
			status:   models.StatusDone,
			expected: "StatusDone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}
