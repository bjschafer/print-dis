// Package config loads settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the whole of the application's configuration.
type Config struct {
	DatabaseURL string
	BaseURL     *url.URL
	ListenAddr  string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	AdminGroup       string

	// DevUser, when set, bypasses OIDC and signs everyone in as an admin with this email.
	DevUser string

	SpoolmanURL       string
	PreviewAllowHosts []string

	FilesBackend      string // "", "s3" or "dir"
	FilesDir          string
	S3Endpoint        string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	UploadMaxBytes    int64
}

// FromEnv reads configuration using getenv (os.Getenv in production).
func FromEnv(getenv func(string) string) (*Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	c := &Config{
		DatabaseURL:       getenv("DATABASE_URL"),
		ListenAddr:        getenv("LISTEN_ADDR"),
		OIDCIssuer:        getenv("OIDC_ISSUER"),
		OIDCClientID:      getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:  getenv("OIDC_CLIENT_SECRET"),
		AdminGroup:        getenv("ADMIN_GROUP"),
		DevUser:           strings.ToLower(strings.TrimSpace(getenv("DEV_USER"))),
		SpoolmanURL:       strings.TrimRight(getenv("SPOOLMAN_URL"), "/"),
		PreviewAllowHosts: splitList(getenv("PREVIEW_ALLOW_HOSTS")),
		FilesBackend:      getenv("FILES_BACKEND"),
		FilesDir:          getenv("FILES_DIR"),
		S3Endpoint:        getenv("S3_ENDPOINT"),
		S3Bucket:          getenv("S3_BUCKET"),
		S3AccessKeyID:     getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey: getenv("S3_SECRET_ACCESS_KEY"),
		S3Region:          getenv("S3_REGION"),
		UploadMaxBytes:    200 << 20,
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if raw := getenv("BASE_URL"); raw == "" {
		errs = append(errs, errors.New("BASE_URL is required"))
	} else if u, err := url.Parse(strings.TrimRight(raw, "/")); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("BASE_URL %q is not an absolute URL", raw))
	} else {
		c.BaseURL = u
	}
	if c.DevUser != "" {
		if c.BaseURL != nil && !isLocalhost(c.BaseURL) {
			errs = append(errs, errors.New("DEV_USER may only be set when BASE_URL is a localhost origin"))
		}
	} else if c.OIDCIssuer == "" || c.OIDCClientID == "" || c.OIDCClientSecret == "" {
		errs = append(errs, errors.New("OIDC_ISSUER, OIDC_CLIENT_ID and OIDC_CLIENT_SECRET are required unless DEV_USER is set"))
	}
	if raw := getenv("UPLOAD_MAX_BYTES"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("UPLOAD_MAX_BYTES %q is not a positive integer", raw))
		} else {
			c.UploadMaxBytes = n
		}
	}
	switch c.FilesBackend {
	case "":
	case "dir":
		if c.FilesDir == "" {
			errs = append(errs, errors.New("FILES_DIR is required when FILES_BACKEND=dir"))
		}
	case "s3":
		if c.S3Endpoint == "" || c.S3Bucket == "" || c.S3AccessKeyID == "" || c.S3SecretAccessKey == "" {
			errs = append(errs, errors.New("S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are required when FILES_BACKEND=s3"))
		}
	default:
		errs = append(errs, fmt.Errorf("FILES_BACKEND %q must be s3, dir or empty", c.FilesBackend))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

// Secure reports whether the public origin is https, which decides cookie flags.
func (c *Config) Secure() bool { return c.BaseURL.Scheme == "https" }

func isLocalhost(u *url.URL) bool {
	h := u.Hostname()
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}
