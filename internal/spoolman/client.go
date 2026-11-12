// Package spoolman is a read-only client for Spoolman's filament and spool
// lists, with a small in-memory cache. print-dis never writes to Spoolman.
package spoolman

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	fetchTimeout = 5 * time.Second
	cacheTTL     = 5 * time.Minute
	failBackoff  = 15 * time.Second
	maxBody      = 32 << 20
)

// FilamentOption is one pickable filament.
type FilamentOption struct {
	ID       int
	Vendor   string
	Name     string
	Material string
	ColorHex string   // "#rrggbb", or "" when unknown; the first of Hexes
	Hexes    []string // every color of a multi-color filament, else just ColorHex
	Label    string   // "Vendor Name"
	InStock  bool     // any unarchived spool has weight left
	Grams    float64  // weight left across unarchived spools
}

// MaterialGroup is the options of one material, sorted by label.
type MaterialGroup struct {
	Material string
	Options  []FilamentOption
}

// GroupByMaterial groups options by material (sorted); within a group,
// filaments in stock come first, then by label.
func GroupByMaterial(opts []FilamentOption) []MaterialGroup {
	idx := map[string]int{}
	var groups []MaterialGroup
	for _, o := range opts {
		m := strings.ToUpper(strings.TrimSpace(o.Material))
		i, ok := idx[m]
		if !ok {
			i = len(groups)
			idx[m] = i
			groups = append(groups, MaterialGroup{Material: m})
		}
		groups[i].Options = append(groups[i].Options, o)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Material < groups[j].Material })
	for _, g := range groups {
		sort.SliceStable(g.Options, func(i, j int) bool {
			a, b := g.Options[i], g.Options[j]
			if a.InStock != b.InStock {
				return a.InStock
			}
			return strings.ToLower(a.Label) < strings.ToLower(b.Label)
		})
	}
	return groups
}

// Client reads from one Spoolman instance. It is safe for concurrent use.
type Client struct {
	base string
	hc   *http.Client
	now  func() time.Time
	ttl  time.Duration

	mu       sync.Mutex
	opts     []FilamentOption
	fetched  time.Time
	have     bool
	lastErr  error
	failedAt time.Time
}

// New returns a client for the Spoolman at baseURL (no trailing /api/v1).
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		hc:   &http.Client{Timeout: fetchTimeout},
		now:  time.Now,
		ttl:  cacheTTL,
	}
}

// Filaments returns the filament list, from cache when fresh. When a refresh
// fails it returns the last good list; with none it returns the error.
func (c *Client) Filaments(ctx context.Context) ([]FilamentOption, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.have && c.now().Sub(c.fetched) < c.ttl {
		return c.opts, nil
	}
	// While down with nothing cached, do not make every page load wait out
	// the timeout: replay the last error for a short while.
	if !c.have && c.lastErr != nil && c.now().Sub(c.failedAt) < failBackoff {
		return nil, c.lastErr
	}
	opts, err := c.fetch(ctx)
	if err != nil {
		if c.have {
			return c.opts, nil
		}
		c.lastErr, c.failedAt = err, c.now()
		return nil, err
	}
	c.opts, c.fetched, c.have, c.lastErr = opts, c.now(), true, nil
	return opts, nil
}

func (c *Client) fetch(ctx context.Context) ([]FilamentOption, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	var filaments []Filament
	if err := c.get(ctx, "/api/v1/filament", &filaments); err != nil {
		return nil, err
	}
	var spools []Spool
	if err := c.get(ctx, "/api/v1/spool", &spools); err != nil {
		return nil, err
	}
	return buildOptions(filaments, spools), nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("spoolman %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("spoolman %s: status %d", path, res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(v); err != nil {
		return fmt.Errorf("spoolman %s: bad response: %w", path, err)
	}
	return nil
}

func buildOptions(filaments []Filament, spools []Spool) []FilamentOption {
	grams := map[int]float64{}
	for _, s := range spools {
		if !s.Archived && s.RemainingWeight > 0 {
			grams[s.Filament.ID] += s.RemainingWeight
		}
	}
	opts := make([]FilamentOption, 0, len(filaments))
	for _, f := range filaments {
		var hexes []string
		for part := range strings.SplitSeq(f.MultiColorHexes, ",") {
			if h := NormalizeHex(part); h != "" {
				hexes = append(hexes, h)
			}
		}
		if len(hexes) == 0 {
			if h := NormalizeHex(f.ColorHex); h != "" {
				hexes = []string{h}
			}
		}
		hex := ""
		if len(hexes) > 0 {
			hex = hexes[0]
		}
		vendor := strings.TrimSpace(f.Vendor.Name)
		name := strings.TrimSpace(f.Name)
		opts = append(opts, FilamentOption{
			ID:       f.ID,
			Vendor:   vendor,
			Name:     name,
			Material: strings.ToUpper(strings.TrimSpace(f.Material)),
			ColorHex: hex,
			Hexes:    hexes,
			Label:    strings.TrimSpace(vendor + " " + name),
			InStock:  grams[f.ID] > 0,
			Grams:    grams[f.ID],
		})
	}
	sort.SliceStable(opts, func(i, j int) bool {
		if opts[i].Material != opts[j].Material {
			return opts[i].Material < opts[j].Material
		}
		return strings.ToLower(opts[i].Label) < strings.ToLower(opts[j].Label)
	})
	return opts
}

// NormalizeHex returns "#rrggbb" for a 6 or 8 digit hex color with or
// without "#", or "" when s is not one.
func NormalizeHex(s string) string {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "#")
	if len(s) != 6 && len(s) != 8 {
		return ""
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return "#" + s[:6]
}
