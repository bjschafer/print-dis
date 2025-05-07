package spoolman

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFilament(t *testing.T) {
	tests := []struct {
		name    string
		want    *Filament
		wantErr error
	}{
		{
			name: "found filament",
			want: &Filament{
				Id:         269,
				Registered: "2025-04-21T22:32:17Z",
				Name:       "ASA-GF Zinc Yellow",
				Vendor: Vendor{
					Id:         24,
					Registered: "2025-04-21T22:29:06Z",
					Name:       "Decent",
					Extra:      nil,
				},
				Material:             "ASA-GF",
				Density:              1.1,
				Diameter:             1.75,
				Weight:               1000.0,
				SettingsExtruderTemp: 260,
				SettingsBedTemp:      105,
				ColorHex:             "f4d469",
				Extra:                nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expected, err := json.Marshal(tt.want)
				if err != nil {
					t.Error(err)
				}

				_, err = w.Write(expected)
				if err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()

			c := Client{
				endpoint: srv.URL,
				client:   *srv.Client(),
			}

			got, err := c.GetFilament(context.Background(), 42)
			if tt.wantErr != nil && assert.Error(t, err) {
				assert.Equal(t, tt.wantErr, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
