// Package preview fetches a link's title and thumbnail for the new-request
// form. The server makes these requests on behalf of a signed-in user, so
// every connection goes through an SSRF guard (guard.go).
package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const (
	// MaxImageBytes caps a stored thumbnail.
	MaxImageBytes = 512 << 10
	// MaxHTMLBytes caps how much of a page is read.
	MaxHTMLBytes  = 2 << 20
	maxTitleRunes = 200
	maxRedirects  = 5
	userAgent     = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"
)

// ErrNothing means the page was fetched but had no title or image.
var ErrNothing = errors.New("preview: page has no title or image")

// Preview is what a link told us about itself. Image is nil when there was
// none, or it was too big, or not a supported image.
type Preview struct {
	Title     string
	Image     []byte
	ImageType string
}

// Fetcher downloads previews through the SSRF guard.
type Fetcher struct {
	client  *http.Client
	timeout time.Duration
}

// Option adjusts a Fetcher. Production uses none except WithAllowHosts.
type Option func(*config)

type config struct {
	allowHosts    map[string]bool
	allowLoopback bool
	timeout       time.Duration
}

// WithAllowHosts lets the named hosts (lower-case hostnames or IP literals)
// connect to private addresses. Only a URL whose own host matches is exempt:
// a redirect to any other host is judged on its own.
func WithAllowHosts(hosts ...string) Option {
	return func(c *config) {
		for _, h := range hosts {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
				c.allowHosts[h] = true
			}
		}
	}
}

// WithAllowLoopback permits loopback addresses for everyone. For tests that
// serve from httptest on 127.0.0.1; never used in production.
func WithAllowLoopback() Option { return func(c *config) { c.allowLoopback = true } }

// WithTimeout sets the overall time allowed for a Fetch (default 8s).
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// New builds a Fetcher with strict defaults.
func New(opts ...Option) *Fetcher {
	c := &config{allowHosts: map[string]bool{}, timeout: 8 * time.Second}
	for _, o := range opts {
		o(c)
	}
	tr := &http.Transport{
		Proxy:                 nil, // never via an environment proxy: the guard must see the real peer
		DialContext:           guardedDialer(c),
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: c.timeout,
		DisableKeepAlives:     true,
	}
	return &Fetcher{
		timeout: c.timeout,
		client: &http.Client{
			Transport: tr,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > maxRedirects {
					return errors.New("too many redirects")
				}
				return checkScheme(req.URL)
			},
		},
	}
}

func checkScheme(u *url.URL) error {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("preview: unsupported URL %q", u.Redacted())
	}
	return nil
}

// Fetch reads the page at rawURL. A page that loads but whose image does not
// still succeeds, with no image.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Preview, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("preview: bad URL: %w", err)
	}
	if err := checkScheme(u); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	res, err := f.get(ctx, u.String(), "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("preview: %s answered %s", u.Hostname(), res.Status)
	}
	if ct := strings.ToLower(res.Header.Get("Content-Type")); ct != "" && !strings.Contains(ct, "html") {
		return nil, fmt.Errorf("preview: %s is not a web page (%s)", u.Hostname(), ct)
	}
	meta := parseHead(io.LimitReader(res.Body, MaxHTMLBytes))
	p := &Preview{Title: meta.title()}
	if ref := strings.TrimSpace(meta.ogImage); ref != "" {
		// res.Request.URL is the final URL after redirects: resolve against it.
		if iu, err := res.Request.URL.Parse(ref); err == nil && checkScheme(iu) == nil {
			p.Image, p.ImageType = f.fetchImage(ctx, iu.String())
		}
	}
	if p.Title == "" && p.Image == nil {
		return nil, ErrNothing
	}
	return p, nil
}

func (f *Fetcher) get(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	return f.client.Do(req)
}

var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// fetchImage returns nil when the image is unavailable, too big, or not a
// supported type. The type is sniffed from the bytes, not taken from headers.
func (f *Fetcher) fetchImage(ctx context.Context, rawURL string) ([]byte, string) {
	res, err := f.get(ctx, rawURL, "image/avif,image/webp,image/png,image/jpeg,image/gif,*/*;q=0.5")
	if err != nil {
		return nil, ""
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK || res.ContentLength > MaxImageBytes {
		return nil, ""
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, MaxImageBytes+1))
	if err != nil || len(b) == 0 || len(b) > MaxImageBytes {
		return nil, ""
	}
	typ, _, _ := strings.Cut(http.DetectContentType(b), ";")
	if !imageTypes[typ] {
		return nil, ""
	}
	return b, typ
}

type headMeta struct {
	ogTitle, ogImage, docTitle string
}

func (m headMeta) title() string {
	t := m.ogTitle
	if strings.TrimSpace(t) == "" {
		t = m.docTitle
	}
	return cleanTitle(t)
}

func cleanTitle(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxTitleRunes {
		s = string([]rune(s)[:maxTitleRunes])
	}
	return s
}

// parseHead tokenizes the page, collecting Open Graph tags and <title>.
func parseHead(r io.Reader) headMeta {
	var m headMeta
	z := html.NewTokenizer(r)
	inTitle := false
	var title bytes.Buffer
	for {
		switch z.Next() {
		case html.ErrorToken:
			if m.docTitle == "" {
				m.docTitle = title.String()
			}
			return m
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			switch t.Data {
			case "meta":
				var key, val string
				for _, a := range t.Attr {
					switch a.Key {
					case "property", "name":
						if key == "" {
							key = strings.ToLower(a.Val)
						}
					case "content":
						val = a.Val
					}
				}
				switch key {
				case "og:title":
					if m.ogTitle == "" {
						m.ogTitle = val
					}
				case "og:image", "og:image:url":
					if m.ogImage == "" {
						m.ogImage = val
					}
				}
			case "title":
				inTitle = m.docTitle == "" && title.Len() == 0
			}
		case html.TextToken:
			if inTitle {
				title.Write(z.Text())
			}
		case html.EndTagToken:
			if n, _ := z.TagName(); string(n) == "title" && inTitle {
				inTitle = false
				m.docTitle = title.String()
			}
		}
		if m.ogTitle != "" && m.ogImage != "" {
			return m
		}
	}
}
