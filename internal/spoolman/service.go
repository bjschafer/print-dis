package spoolman

import (
	"context"
	"log/slog"
	"sync"
)

// Service handles Spoolman integration
type Service struct {
	client *Client
	mu     sync.RWMutex
}

// NewService creates a new Spoolman service
func NewService(client *Client) *Service {
	return &Service{
		client: client,
	}
}

// GetSpool retrieves a spool by ID
func (s *Service) GetSpool(ctx context.Context, id int) (*Spool, error) {
	slog.Debug("getting spool from Spoolman", "id", id)
	s.mu.RLock()
	defer s.mu.RUnlock()

	spool, err := s.client.GetSpool(ctx, id)
	if err != nil {
		slog.Error("failed to get spool from Spoolman", "id", id, "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved spool from Spoolman", "id", id)
	return spool, nil
}

// GetSpools retrieves all available spools
func (s *Service) GetSpools(ctx context.Context) ([]Spool, error) {
	slog.Debug("getting all spools from Spoolman")
	s.mu.RLock()
	defer s.mu.RUnlock()

	spools, err := s.client.GetSpools(ctx)
	if err != nil {
		slog.Error("failed to get spools from Spoolman", "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved spools from Spoolman", "count", len(spools))
	return spools, nil
}

// GetMaterials retrieves all unique materials from Spoolman
func (s *Service) GetMaterials(ctx context.Context) ([]string, error) {
	slog.Debug("getting materials from Spoolman")
	s.mu.RLock()
	defer s.mu.RUnlock()

	materials, err := s.client.GetMaterials(ctx)
	if err != nil {
		slog.Error("failed to get materials from Spoolman", "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved materials from Spoolman", "count", len(materials))
	return materials, nil
}

// GetFilament retrieves filament information by ID
func (s *Service) GetFilament(ctx context.Context, id int) (*Filament, error) {
	slog.Debug("getting filament from Spoolman", "id", id)
	s.mu.RLock()
	defer s.mu.RUnlock()

	filament, err := s.client.GetFilament(ctx, id)
	if err != nil {
		slog.Error("failed to get filament from Spoolman", "id", id, "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved filament from Spoolman", "id", id)
	return filament, nil
}
