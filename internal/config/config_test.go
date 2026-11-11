package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestFromEnv(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":       "postgres://x",
		"BASE_URL":           "https://prints.example.com/",
		"OIDC_ISSUER":        "https://login.example.com",
		"OIDC_CLIENT_ID":     "id",
		"OIDC_CLIENT_SECRET": "secret",
	}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"valid", base, ""},
		{"missing db", with("DATABASE_URL", ""), "DATABASE_URL"},
		{"relative base", with("BASE_URL", "prints"), "BASE_URL"},
		{"missing oidc", with("OIDC_CLIENT_SECRET", ""), "OIDC_ISSUER"},
		{"dev user on public origin", with("DEV_USER", "me@example.com"), "localhost"},
		{"dev user on localhost", with("DEV_USER", "me@example.com", "BASE_URL", "http://localhost:8080", "OIDC_ISSUER", ""), ""},
		{"dev user on 127.0.0.1", with("DEV_USER", "me@example.com", "BASE_URL", "http://127.0.0.1:8080"), ""},
		{"bad backend", with("FILES_BACKEND", "nfs"), "FILES_BACKEND"},
		{"dir without dir", with("FILES_BACKEND", "dir"), "FILES_DIR"},
		{"s3 without creds", with("FILES_BACKEND", "s3", "S3_ENDPOINT", "https://s3"), "S3_BUCKET"},
		{"bad max", with("UPLOAD_MAX_BYTES", "-1"), "UPLOAD_MAX_BYTES"},
		{"bad accent", with("ACCENT_COLOR", "green"), "ACCENT_COLOR"},
		{"accent too long", with("ACCENT_COLOR", "#12345678"), "ACCENT_COLOR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := FromEnv(env(tt.env))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if c.ListenAddr != ":8080" || c.UploadMaxBytes != 200<<20 {
					t.Fatalf("defaults not applied: %+v", c)
				}
				if strings.HasSuffix(c.BaseURL.String(), "/") {
					t.Fatalf("trailing slash kept: %s", c.BaseURL)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAccentColor(t *testing.T) {
	for in, want := range map[string]string{"": "", "#00AE42": "#00ae42", "3b82f6": "#3b82f6", "#f80": "#ff8800", " #abc ": "#aabbcc"} {
		m := map[string]string{"DATABASE_URL": "postgres://x", "BASE_URL": "http://localhost:8080", "DEV_USER": "me@example.com", "ACCENT_COLOR": in}
		c, err := FromEnv(env(m))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if c.AccentColor != want {
			t.Errorf("%q: got %q, want %q", in, c.AccentColor, want)
		}
	}
}
