package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/database"
	"github.com/bjschafer/print-dis/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDBClient is a mock implementation of database.DBClient
type MockDBClient struct {
	mock.Mock
}

// MockTx is a mock implementation of database.Tx
type MockTx struct {
	mock.Mock
}

// Transaction methods
func (m *MockTx) Commit() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTx) Rollback() error {
	args := m.Called()
	return args.Error(0)
}

// User operations
func (m *MockTx) CreateUser(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockTx) GetUser(ctx context.Context, id string) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockTx) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockTx) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockTx) UpdateUser(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// PrintRequest operations
func (m *MockTx) CreatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockTx) GetPrintRequest(ctx context.Context, id string) (*models.PrintRequest, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PrintRequest), args.Error(1)
}

func (m *MockTx) UpdatePrintRequest(ctx context.Context, request *models.PrintRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
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

func (m *MockDBClient) ListPrintRequestsByUserID(ctx context.Context, userID string) ([]*models.PrintRequest, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
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

// User operations
func (m *MockDBClient) CreateUser(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}
func (m *MockDBClient) GetUser(ctx context.Context, id string) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockDBClient) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockDBClient) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockDBClient) UpdateUser(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}
func (m *MockDBClient) DeleteUser(ctx context.Context, id string) error {
	return nil
}
func (m *MockDBClient) ListUsers(ctx context.Context) ([]*models.User, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.User), args.Error(1)
}

// Close implements the database.DBClient interface
func (m *MockDBClient) Close() error {
	return nil
}

// BeginTx starts a new transaction
func (m *MockDBClient) BeginTx(ctx context.Context) (database.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(database.Tx), args.Error(1)
}

func (m *MockDBClient) GetDB() *sql.DB {
	return nil // Mock implementation - not needed for tests
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
			name:          "Valid Transition: PendingApproval to Enqueued",
			currentStatus: models.StatusPendingApproval,
			newStatus:     models.StatusEnqueued,
			shouldError:   false,
		},
		{
			name:          "Valid Transition: Enqueued to InProgress",
			currentStatus: models.StatusEnqueued,
			newStatus:     models.StatusInProgress,
			shouldError:   false,
		},
		{
			name:          "Valid Transition: InProgress to Done",
			currentStatus: models.StatusInProgress,
			newStatus:     models.StatusDone,
			shouldError:   false,
		},
		{
			name:          "Invalid Transition: PendingApproval to InProgress",
			currentStatus: models.StatusPendingApproval,
			newStatus:     models.StatusInProgress,
			shouldError:   true,
		},
		{
			name:          "Invalid Transition: PendingApproval to Done",
			currentStatus: models.StatusPendingApproval,
			newStatus:     models.StatusDone,
			shouldError:   true,
		},
		{
			name:          "Invalid Transition: Enqueued to Done",
			currentStatus: models.StatusEnqueued,
			newStatus:     models.StatusDone,
			shouldError:   true,
		},
		{
			name:          "Invalid Transition: Done to InProgress",
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

			// Create a mock transaction
			mockTx := new(MockTx)

			// Set up the mock for BeginTx
			mockDB.On("BeginTx", ctx).Return(mockTx, nil)

			// Set up the mock for GetPrintRequest within the transaction
			mockTx.On("GetPrintRequest", ctx, testRequest.ID).Return(&testRequest, nil)

			if !tt.shouldError {
				// Set up UpdatePrintRequest mock for valid transitions
				mockTx.On("UpdatePrintRequest", ctx, mock.MatchedBy(func(req *models.PrintRequest) bool {
					return req.ID == testRequest.ID && req.Status == tt.newStatus
				})).Return(nil)

				// Set up transaction commit for valid transitions
				mockTx.On("Commit").Return(nil)
			}

			// Set up transaction rollback for any case (will be called by defer if error occurs)
			mockTx.On("Rollback").Return(nil).Maybe()

			// Try to update the status
			updatedRequest := *request
			updatedRequest.Status = tt.newStatus
			err := service.UpdatePrintRequest(ctx, &updatedRequest)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify all expectations were met
			mockDB.AssertExpectations(t)
			mockTx.AssertExpectations(t)
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
