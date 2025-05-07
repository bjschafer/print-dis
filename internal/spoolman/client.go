package spoolman

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type Client struct {
	endpoint string
	client   http.Client
}

func New(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		client:   *http.DefaultClient,
	}
}

func (c *Client) GetFilament(ctx context.Context, id int) (*Filament, error) {
	u, err := url.JoinPath(c.endpoint, "filament", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	f := new(Filament)
	err = json.Unmarshal(body, f)
	return f, err
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
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	ret := []Filament{}
	err = json.Unmarshal(body, &ret)

	return ret, err
}
