package spoolman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	endpoint string
	client   http.Client
}

func New(endpoint string) *Client {
	// Parse the endpoint URL
	u, err := url.Parse(endpoint)
	if err != nil {
		slog.Error("failed to parse Spoolman endpoint URL", "endpoint", endpoint, "error", err)
		return nil
	}

	// Ensure path ends with /api/v1
	path := u.Path
	if !strings.HasSuffix(path, "/api/v1") {
		path = strings.TrimRight(path, "/") + "/api/v1"
	}
	u.Path = path

	slog.Info("creating new Spoolman client", "endpoint", u.String())
	return &Client{
		endpoint: u.String(),
		client:   *http.DefaultClient,
	}
}

func (c *Client) GetFilament(ctx context.Context, id int) (*Filament, error) {
	u, err := url.JoinPath(c.endpoint, "filament", strconv.Itoa(id))
	if err != nil {
		slog.Error("failed to construct filament URL", "id", id, "error", err)
		return nil, err
	}

	slog.Debug("requesting filament from Spoolman", "url", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		slog.Error("failed to create filament request", "url", u, "error", err)
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("failed to execute filament request", "url", u, "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("unexpected status code from Spoolman", "url", u, "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read filament response body", "url", u, "error", err)
		return nil, err
	}

	f := new(Filament)
	err = json.Unmarshal(body, f)
	if err != nil {
		slog.Error("failed to unmarshal filament response", "url", u, "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved filament", "id", id)
	return f, nil
}

func (c *Client) FindFilaments(ctx context.Context, query *FilamentRequest) ([]Filament, error) {
	u, err := url.JoinPath(c.endpoint, "filament")
	if err != nil {
		return nil, err
	}

	queryBody, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	reqBody := bytes.NewReader(queryBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, reqBody)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	ret := []Filament{}
	err = json.Unmarshal(body, &ret)

	return ret, err
}

func (c *Client) GetSpool(ctx context.Context, id int) (*Spool, error) {
	u, err := url.JoinPath(c.endpoint, "spool", strconv.Itoa(id))
	if err != nil {
		slog.Error("failed to construct spool URL", "id", id, "error", err)
		return nil, err
	}

	slog.Debug("requesting spool from Spoolman", "url", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		slog.Error("failed to create spool request", "url", u, "error", err)
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("failed to execute spool request", "url", u, "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("unexpected status code from Spoolman", "url", u, "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read spool response body", "url", u, "error", err)
		return nil, err
	}

	s := new(Spool)
	err = json.Unmarshal(body, s)
	if err != nil {
		slog.Error("failed to unmarshal spool response", "url", u, "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved spool", "id", id)
	return s, nil
}

func (c *Client) GetSpools(ctx context.Context) ([]Spool, error) {
	u, err := url.JoinPath(c.endpoint, "spool")
	if err != nil {
		slog.Error("failed to construct spools URL", "error", err)
		return nil, err
	}

	slog.Debug("requesting spools from Spoolman", "url", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		slog.Error("failed to create spools request", "url", u, "error", err)
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("failed to execute spools request", "url", u, "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("unexpected status code from Spoolman", "url", u, "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read spools response body", "url", u, "error", err)
		return nil, err
	}

	var spools []Spool
	err = json.Unmarshal(body, &spools)
	if err != nil {
		slog.Error("failed to unmarshal spools response", "url", u, "error", err)
		return nil, err
	}
	slog.Debug("successfully retrieved spools", "count", len(spools))
	return spools, nil
}

func (c *Client) GetMaterials(ctx context.Context) ([]string, error) {
	u, err := url.JoinPath(c.endpoint, "filament")
	if err != nil {
		slog.Error("failed to construct materials URL", "error", err)
		return nil, err
	}

	slog.Debug("requesting materials from Spoolman", "url", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		slog.Error("failed to create materials request", "url", u, "error", err)
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("failed to execute materials request", "url", u, "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("unexpected status code from Spoolman", "url", u, "status", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read materials response body", "url", u, "error", err)
		return nil, err
	}

	var filaments []Filament
	err = json.Unmarshal(body, &filaments)
	if err != nil {
		slog.Error("failed to unmarshal materials response", "url", u, "error", err)
		return nil, err
	}

	// Extract unique materials
	materials := make(map[string]struct{})
	for _, f := range filaments {
		if f.Material != "" {
			materials[f.Material] = struct{}{}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(materials))
	for m := range materials {
		result = append(result, m)
	}

	slog.Debug("successfully retrieved materials", "count", len(result))
	return result, nil
}
