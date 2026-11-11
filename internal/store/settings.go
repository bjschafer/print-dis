package store

import (
	"context"

	"github.com/bjschafer/print-dis/internal/core"
)

// GetSettings returns the single settings row.
func (s *Store) GetSettings(ctx context.Context) (core.Settings, error) {
	var st core.Settings
	err := s.Pool.QueryRow(ctx, `select bambuddy_url, bambuddy_api_key from settings`).
		Scan(&st.BambuddyURL, &st.BambuddyAPIKey)
	return st, err
}

// SaveSettings replaces the settings row.
func (s *Store) SaveSettings(ctx context.Context, st core.Settings) error {
	_, err := s.Pool.Exec(ctx, `update settings set bambuddy_url = $1, bambuddy_api_key = $2`,
		st.BambuddyURL, st.BambuddyAPIKey)
	return err
}
