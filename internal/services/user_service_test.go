package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUserService_RegisterUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful registration", func(t *testing.T) {
		mockDB := &MockDBClient{}
		mockTx := &MockTx{}
		service := NewUserService(mockDB)

		// Setup expectations
		mockDB.On("BeginTx", ctx).Return(mockTx, nil)
		mockTx.On("GetUserByUsername", ctx, "testuser").Return(nil, sql.ErrNoRows)
		mockTx.On("GetUserByEmail", ctx, "test@example.com").Return(nil, sql.ErrNoRows)
		mockTx.On("CreateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)
		mockTx.On("Commit").Return(nil)
		// No rollback expected for successful case

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "test@example.com", "password123")

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "testuser", user.Username)
		assert.Equal(t, "test@example.com", *user.Email)
		assert.True(t, user.Enabled)
		assert.NotEmpty(t, user.ID)
		mockDB.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("registration with username only", func(t *testing.T) {
		mockDB := &MockDBClient{}
		mockTx := &MockTx{}
		service := NewUserService(mockDB)

		// Setup expectations
		mockDB.On("BeginTx", ctx).Return(mockTx, nil)
		mockTx.On("GetUserByUsername", ctx, "testuser").Return(nil, sql.ErrNoRows)
		mockTx.On("CreateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)
		mockTx.On("Commit").Return(nil)
		// No rollback expected for successful case

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "", "password123")

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "testuser", user.Username)
		assert.Nil(t, user.Email)
		mockDB.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("empty username", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Execute
		user, err := service.RegisterUser(ctx, "", "test@example.com", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "username is required")
	})

	t.Run("empty password", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "test@example.com", "")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "password is required")
	})

	t.Run("username already exists", func(t *testing.T) {
		mockDB := &MockDBClient{}
		mockTx := &MockTx{}
		service := NewUserService(mockDB)

		existingUser := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
		}

		// Setup expectations
		mockDB.On("BeginTx", ctx).Return(mockTx, nil)
		mockTx.On("GetUserByUsername", ctx, "testuser").Return(existingUser, nil)
		mockTx.On("Rollback").Return(nil).Maybe()

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "test@example.com", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "username already exists")
		mockDB.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("email already exists", func(t *testing.T) {
		mockDB := &MockDBClient{}
		mockTx := &MockTx{}
		service := NewUserService(mockDB)

		existingUser := &models.User{
			ID:    uuid.New().String(),
			Email: stringPtr("test@example.com"),
		}

		// Setup expectations
		mockDB.On("BeginTx", ctx).Return(mockTx, nil)
		mockTx.On("GetUserByUsername", ctx, "testuser").Return(nil, sql.ErrNoRows)
		mockTx.On("GetUserByEmail", ctx, "test@example.com").Return(existingUser, nil)
		mockTx.On("Rollback").Return(nil).Maybe()

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "test@example.com", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "email already exists")
		mockDB.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("database error on begin transaction", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		dbErr := errors.New("database connection error")
		mockDB.On("BeginTx", ctx).Return(nil, dbErr)

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "test@example.com", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "failed to begin transaction")
		mockDB.AssertExpectations(t)
	})

	t.Run("database error on create user", func(t *testing.T) {
		mockDB := &MockDBClient{}
		mockTx := &MockTx{}
		service := NewUserService(mockDB)

		dbErr := errors.New("database insert error")

		// Setup expectations
		mockDB.On("BeginTx", ctx).Return(mockTx, nil)
		mockTx.On("GetUserByUsername", ctx, "testuser").Return(nil, sql.ErrNoRows)
		mockTx.On("GetUserByEmail", ctx, "test@example.com").Return(nil, sql.ErrNoRows)
		mockTx.On("CreateUser", ctx, mock.AnythingOfType("*models.User")).Return(dbErr)
		mockTx.On("Rollback").Return(nil).Maybe()

		// Execute
		user, err := service.RegisterUser(ctx, "testuser", "test@example.com", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "failed to create user")
		mockDB.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})
}

func TestUserService_AuthenticateUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful authentication", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Create a user with a known password
		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}
		err := user.SetPassword("password123")
		assert.NoError(t, err)

		// Setup expectations
		mockDB.On("GetUserByUsername", ctx, "testuser").Return(user, nil)

		// Execute
		authenticatedUser, err := service.AuthenticateUser(ctx, "testuser", "password123")

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, authenticatedUser)
		assert.Equal(t, user.ID, authenticatedUser.ID)
		assert.Equal(t, user.Username, authenticatedUser.Username)
		mockDB.AssertExpectations(t)
	})

	t.Run("user not found", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Setup expectations
		mockDB.On("GetUserByUsername", ctx, "nonexistent").Return(nil, sql.ErrNoRows)

		// Execute
		user, err := service.AuthenticateUser(ctx, "nonexistent", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "invalid credentials")
		mockDB.AssertExpectations(t)
	})

	t.Run("user disabled", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  false,
		}
		err := user.SetPassword("password123")
		assert.NoError(t, err)

		// Setup expectations
		mockDB.On("GetUserByUsername", ctx, "testuser").Return(user, nil)

		// Execute
		authenticatedUser, err := service.AuthenticateUser(ctx, "testuser", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, authenticatedUser)
		assert.Contains(t, err.Error(), "account is disabled")
		mockDB.AssertExpectations(t)
	})

	t.Run("wrong password", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}
		err := user.SetPassword("password123")
		assert.NoError(t, err)

		// Setup expectations
		mockDB.On("GetUserByUsername", ctx, "testuser").Return(user, nil)

		// Execute
		authenticatedUser, err := service.AuthenticateUser(ctx, "testuser", "wrongpassword")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, authenticatedUser)
		assert.Contains(t, err.Error(), "invalid credentials")
		mockDB.AssertExpectations(t)
	})

	t.Run("database error", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		dbErr := errors.New("database connection error")
		mockDB.On("GetUserByUsername", ctx, "testuser").Return(nil, dbErr)

		// Execute
		user, err := service.AuthenticateUser(ctx, "testuser", "password123")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "failed to get user")
		mockDB.AssertExpectations(t)
	})
}

func TestUserService_GetUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful get user", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}

		// Setup expectations
		mockDB.On("GetUser", ctx, user.ID).Return(user, nil)

		// Execute
		foundUser, err := service.GetUser(ctx, user.ID)

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, foundUser)
		assert.Equal(t, user.ID, foundUser.ID)
		assert.Equal(t, user.Username, foundUser.Username)
		mockDB.AssertExpectations(t)
	})

	t.Run("user not found", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		userID := uuid.New().String()
		mockDB.On("GetUser", ctx, userID).Return(nil, sql.ErrNoRows)

		// Execute
		user, err := service.GetUser(ctx, userID)

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "user not found")
		mockDB.AssertExpectations(t)
	})

	t.Run("database error", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		userID := uuid.New().String()
		dbErr := errors.New("database connection error")
		mockDB.On("GetUser", ctx, userID).Return(nil, dbErr)

		// Execute
		user, err := service.GetUser(ctx, userID)

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "failed to get user")
		mockDB.AssertExpectations(t)
	})
}

func TestUserService_ChangePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("successful password change", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}
		err := user.SetPassword("oldpassword")
		assert.NoError(t, err)

		// Setup expectations
		mockDB.On("GetUser", ctx, user.ID).Return(user, nil)
		mockDB.On("UpdateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)

		// Execute
		err = service.ChangePassword(ctx, user.ID, "oldpassword", "newpassword123")

		// Verify
		assert.NoError(t, err)
		assert.True(t, user.CheckPassword("newpassword123"))
		assert.False(t, user.CheckPassword("oldpassword"))
		mockDB.AssertExpectations(t)
	})

	t.Run("user not found", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		userID := uuid.New().String()
		mockDB.On("GetUser", ctx, userID).Return(nil, sql.ErrNoRows)

		// Execute
		err := service.ChangePassword(ctx, userID, "oldpassword", "newpassword123")

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
		mockDB.AssertExpectations(t)
	})

	t.Run("incorrect current password", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}
		err := user.SetPassword("correctpassword")
		assert.NoError(t, err)

		// Setup expectations
		mockDB.On("GetUser", ctx, user.ID).Return(user, nil)

		// Execute
		err = service.ChangePassword(ctx, user.ID, "wrongpassword", "newpassword123")

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "current password is incorrect")
		mockDB.AssertExpectations(t)
	})

	t.Run("database error on update", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}
		err := user.SetPassword("oldpassword")
		assert.NoError(t, err)

		dbErr := errors.New("database update error")
		mockDB.On("GetUser", ctx, user.ID).Return(user, nil)
		mockDB.On("UpdateUser", ctx, mock.AnythingOfType("*models.User")).Return(dbErr)

		// Execute
		err = service.ChangePassword(ctx, user.ID, "oldpassword", "newpassword123")

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update user")
		mockDB.AssertExpectations(t)
	})
}

func TestUserService_DisableUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful disable user", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  true,
		}

		// Setup expectations
		mockDB.On("GetUser", ctx, user.ID).Return(user, nil)
		mockDB.On("UpdateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)

		// Execute
		err := service.DisableUser(ctx, user.ID)

		// Verify
		assert.NoError(t, err)
		assert.False(t, user.Enabled)
		mockDB.AssertExpectations(t)
	})

	t.Run("user not found", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		userID := uuid.New().String()
		mockDB.On("GetUser", ctx, userID).Return(nil, sql.ErrNoRows)

		// Execute
		err := service.DisableUser(ctx, userID)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
		mockDB.AssertExpectations(t)
	})
}

func TestUserService_EnableUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful enable user", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		user := &models.User{
			ID:       uuid.New().String(),
			Username: "testuser",
			Enabled:  false,
		}

		// Setup expectations
		mockDB.On("GetUser", ctx, user.ID).Return(user, nil)
		mockDB.On("UpdateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)

		// Execute
		err := service.EnableUser(ctx, user.ID)

		// Verify
		assert.NoError(t, err)
		assert.True(t, user.Enabled)
		mockDB.AssertExpectations(t)
	})
}

func TestUserService_ListUsers(t *testing.T) {
	ctx := context.Background()

	t.Run("successful list users", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		users := []*models.User{
			{
				ID:       uuid.New().String(),
				Username: "user1",
				Enabled:  true,
			},
			{
				ID:       uuid.New().String(),
				Username: "user2",
				Enabled:  false,
			},
		}

		// Setup expectations
		mockDB.On("ListUsers", ctx).Return(users, nil)

		// Execute
		foundUsers, err := service.ListUsers(ctx)

		// Verify
		assert.NoError(t, err)
		assert.Len(t, foundUsers, 2)
		assert.Equal(t, users[0].ID, foundUsers[0].ID)
		assert.Equal(t, users[1].ID, foundUsers[1].ID)
		mockDB.AssertExpectations(t)
	})

	t.Run("empty user list", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Setup expectations
		mockDB.On("ListUsers", ctx).Return([]*models.User{}, nil)

		// Execute
		users, err := service.ListUsers(ctx)

		// Verify
		assert.NoError(t, err)
		assert.Empty(t, users)
		mockDB.AssertExpectations(t)
	})

	t.Run("database error", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		dbErr := errors.New("database connection error")
		mockDB.On("ListUsers", ctx).Return(nil, dbErr)

		// Execute
		users, err := service.ListUsers(ctx)

		// Verify
		assert.Error(t, err)
		assert.Nil(t, users)
		assert.Contains(t, err.Error(), "failed to list users")
		mockDB.AssertExpectations(t)
	})
}

func TestUserService_CreateSession(t *testing.T) {
	ctx := context.Background()

	t.Run("successful session creation", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		userID := uuid.New().String()
		duration := 24 * time.Hour

		// Execute
		session, err := service.CreateSession(ctx, userID, duration)

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, session)
		assert.Equal(t, userID, session.UserID)
		assert.NotEmpty(t, session.ID)
		assert.NotEmpty(t, session.SessionToken)
		assert.True(t, session.ExpiresAt.After(time.Now()))
		assert.True(t, session.ExpiresAt.Before(time.Now().Add(duration+time.Minute)))
	})
}

func TestUserService_FindOrCreateUserFromOIDC(t *testing.T) {
	ctx := context.Background()

	t.Run("find existing user by email", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		existingUser := &models.User{
			ID:       uuid.New().String(),
			Username: "existinguser",
			Email:    stringPtr("test@example.com"),
			Enabled:  true,
		}

		// Setup expectations
		mockDB.On("GetUserByEmail", ctx, "test@example.com").Return(existingUser, nil)

		// Execute
		user, err := service.FindOrCreateUserFromOIDC(ctx, "google", "12345", "test@example.com", "Test User")

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, existingUser.ID, user.ID)
		assert.Equal(t, existingUser.Username, user.Username)
		mockDB.AssertExpectations(t)
	})

	t.Run("create new user from OIDC with email", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Setup expectations
		mockDB.On("GetUserByEmail", ctx, "new@example.com").Return(nil, sql.ErrNoRows)
		mockDB.On("CreateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)

		// Execute
		user, err := service.FindOrCreateUserFromOIDC(ctx, "google", "12345", "new@example.com", "New User")

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "new@example.com", user.Username)
		assert.Equal(t, "new@example.com", *user.Email)
		assert.Equal(t, "New User", *user.DisplayName)
		assert.True(t, user.Enabled)
		mockDB.AssertExpectations(t)
	})

	t.Run("create new user from OIDC without email", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		// Setup expectations
		mockDB.On("CreateUser", ctx, mock.AnythingOfType("*models.User")).Return(nil)

		// Execute
		user, err := service.FindOrCreateUserFromOIDC(ctx, "google", "12345", "", "New User")

		// Verify
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "google_12345", user.Username)
		assert.Nil(t, user.Email)
		assert.Equal(t, "New User", *user.DisplayName)
		assert.True(t, user.Enabled)
		mockDB.AssertExpectations(t)
	})

	t.Run("database error on create user", func(t *testing.T) {
		mockDB := &MockDBClient{}
		service := NewUserService(mockDB)

		dbErr := errors.New("database insert error")
		mockDB.On("GetUserByEmail", ctx, "test@example.com").Return(nil, sql.ErrNoRows)
		mockDB.On("CreateUser", ctx, mock.AnythingOfType("*models.User")).Return(dbErr)

		// Execute
		user, err := service.FindOrCreateUserFromOIDC(ctx, "google", "12345", "test@example.com", "Test User")

		// Verify
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "failed to create user from OIDC")
		mockDB.AssertExpectations(t)
	})
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}