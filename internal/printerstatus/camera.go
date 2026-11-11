package printerstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// streamHeaderTimeout covers a camera that is slow to start: Bambuddy
	// starts ffmpeg against the printer's RTSP feed on the first request.
	streamHeaderTimeout = 20 * time.Second
	// streamMaxAge ends a live view left open in a forgotten tab.
	streamMaxAge = 30 * time.Minute
	// snapshotInterval paces the frames of a stream built from snapshots.
	snapshotInterval = time.Second
	maxSnapshot      = 8 << 20
	// tokenTTL is how long a Bambuddy stream token is reused; Bambuddy's
	// last 60 minutes.
	tokenTTL = 50 * time.Minute
	// failBackoff is how long a failed Bambuddy printer list is reused.
	failBackoff = 15 * time.Second
)

// ErrNoCamera means the printer's source reports no usable camera.
var ErrNoCamera = errors.New("no camera")

// BambuddyPrinter is one printer as Bambuddy lists it.
type BambuddyPrinter struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

// BambuddyPrinters lists the printers Bambuddy knows, for picking one by
// name. A list is reused for a minute, and a failure for 15 seconds, so the
// printers page does not wait on Bambuddy every time it loads.
func (m *Monitor) BambuddyPrinters(ctx context.Context, baseURL, apiKey string) ([]BambuddyPrinter, error) {
	key := baseURL + "\x00" + apiKey
	m.listMu.Lock()
	defer m.listMu.Unlock()
	if c := m.list; c.key == key && m.opts.Now().Before(c.expires) {
		return c.printers, c.err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	var out []BambuddyPrinter
	err := m.getJSON(ctx, strings.TrimRight(baseURL, "/")+"/api/v1/printers/", bambuddyHeader(apiKey), &out)
	ttl := time.Minute
	if err != nil {
		ttl = failBackoff
	}
	m.list = printerList{key: key, printers: out, err: err, expires: m.opts.Now().Add(ttl)}
	return out, err
}

type printerList struct {
	key      string
	printers []BambuddyPrinter
	err      error
	expires  time.Time
}

func bambuddyHeader(apiKey string) http.Header {
	h := http.Header{}
	if apiKey != "" {
		h.Set("X-API-Key", apiKey)
	}
	return h
}

// CameraInfo is how to show a printer's camera.
type CameraInfo struct {
	Name     string
	Rotation int // degrees clockwise: 0, 90, 180 or 270
	FlipH    bool
	FlipV    bool
}

// CameraStream is an open MJPEG stream. The caller must close Body.
type CameraStream struct {
	ContentType string // multipart/x-mixed-replace; boundary=...
	Body        io.ReadCloser
}

// Camera describes the target's camera without opening it.
func (m *Monitor) Camera(ctx context.Context, t Target) (CameraInfo, error) {
	switch {
	case t.MoonrakerURL != "":
		w, err := m.moonrakerWebcam(ctx, t.MoonrakerURL)
		if err != nil {
			return CameraInfo{}, err
		}
		return CameraInfo{Name: w.Name, Rotation: w.Rotation, FlipH: w.FlipH, FlipV: w.FlipV}, nil
	case t.BambuddyID > 0 && t.BambuddyURL != "":
		return CameraInfo{}, nil
	}
	return CameraInfo{}, ErrNoCamera
}

// OpenCamera opens the target's live camera as MJPEG. The stream ends when
// ctx is done, or after half an hour.
func (m *Monitor) OpenCamera(ctx context.Context, t Target) (*CameraStream, error) {
	ctx, cancel := context.WithTimeout(ctx, streamMaxAge)
	var (
		s   *CameraStream
		err error
	)
	switch {
	case t.MoonrakerURL != "":
		s, err = m.openMoonrakerCamera(ctx, t.MoonrakerURL)
	case t.BambuddyID > 0 && t.BambuddyURL != "":
		s, err = m.openBambuddyCamera(ctx, t)
	default:
		err = ErrNoCamera
	}
	if err != nil {
		cancel()
		return nil, err
	}
	s.Body = &cancelOnClose{ReadCloser: s.Body, cancel: cancel}
	return s, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// streamClient has no overall timeout, since a live view runs for minutes;
// connecting and the response headers are bounded instead.
func (m *Monitor) streamClient() *http.Client {
	m.streamOnce.Do(func() {
		if m.opts.StreamClient != nil {
			m.stream = m.opts.StreamClient
			return
		}
		m.stream = &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: fetchTimeout}).DialContext,
			ResponseHeaderTimeout: streamHeaderTimeout,
			IdleConnTimeout:       time.Minute,
		}}
	})
	return m.stream
}

// openMJPEG requests u and returns the body when it is an MJPEG stream.
func (m *Monitor) openMJPEG(ctx context.Context, u string, h http.Header) (*CameraStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if h != nil {
		req.Header = h
	}
	res, err := m.streamClient().Do(req)
	if err != nil {
		return nil, err
	}
	ct := res.Header.Get("Content-Type")
	mt, params, _ := mime.ParseMediaType(ct)
	if res.StatusCode != http.StatusOK || mt != "multipart/x-mixed-replace" || params["boundary"] == "" {
		_ = res.Body.Close()
		return nil, &cameraError{url: redact(u), status: res.StatusCode, contentType: ct}
	}
	return &CameraStream{ContentType: ct, Body: res.Body}, nil
}

type cameraError struct {
	url         string
	status      int
	contentType string
}

func (e *cameraError) Error() string {
	return fmt.Sprintf("%s: status %d, content type %q", e.url, e.status, e.contentType)
}

// refused reports whether err is the camera refusing the credentials.
func refused(err error) bool {
	var ce *cameraError
	return errors.As(err, &ce) && (ce.status == http.StatusUnauthorized || ce.status == http.StatusForbidden)
}

// ---- Moonraker ----

type moonrakerWebcam struct {
	Name        string `json:"name"`
	Enabled     *bool  `json:"enabled"`
	StreamURL   string `json:"stream_url"`
	SnapshotURL string `json:"snapshot_url"`
	FlipH       bool   `json:"flip_horizontal"`
	FlipV       bool   `json:"flip_vertical"`
	Rotation    int    `json:"rotation"`
}

// moonrakerWebcam returns the first enabled webcam Moonraker knows.
func (m *Monitor) moonrakerWebcam(ctx context.Context, base string) (moonrakerWebcam, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	var r struct {
		Result struct {
			Webcams []moonrakerWebcam `json:"webcams"`
		} `json:"result"`
	}
	if err := m.getJSON(ctx, strings.TrimRight(base, "/")+"/server/webcams/list", http.Header{}, &r); err != nil {
		return moonrakerWebcam{}, err
	}
	for _, w := range r.Result.Webcams {
		if (w.Enabled == nil || *w.Enabled) && (w.StreamURL != "" || w.SnapshotURL != "") {
			switch w.Rotation {
			case 90, 180, 270:
			default:
				w.Rotation = 0
			}
			return w, nil
		}
	}
	return moonrakerWebcam{}, ErrNoCamera
}

// webcamURL resolves a webcam address from Moonraker. Relative ones such as
// "/webcam/?action=stream" are served by the printer's web server (the
// nginx in front of Fluidd or Mainsail), so they resolve against the
// Moonraker host without Moonraker's port.
func webcamURL(moonraker, ref string) (string, error) {
	if ref == "" {
		return "", ErrNoCamera
	}
	base, err := url.Parse(moonraker)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	if r.IsAbs() {
		if r.Scheme != "http" && r.Scheme != "https" {
			return "", fmt.Errorf("webcam address %q is not http", ref)
		}
		return r.String(), nil
	}
	host := base.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	root := &url.URL{Scheme: base.Scheme, Host: host, Path: "/"}
	return root.ResolveReference(r).String(), nil
}

// openMoonrakerCamera tries the webcam's stream address as MJPEG first,
// whatever its service says: Fluidd's "MJPEG Adaptive" is an ordinary
// mjpg-streamer that Fluidd happens to poll for snapshots. When the stream
// address is not MJPEG (WebRTC, HLS), it builds a stream from snapshots.
func (m *Monitor) openMoonrakerCamera(ctx context.Context, base string) (*CameraStream, error) {
	w, err := m.moonrakerWebcam(ctx, base)
	if err != nil {
		return nil, err
	}
	if w.StreamURL != "" {
		if u, err := webcamURL(base, w.StreamURL); err == nil {
			if s, err := m.openMJPEG(ctx, u, nil); err == nil {
				return s, nil
			} else if w.SnapshotURL == "" {
				return nil, err
			}
		}
	}
	u, err := webcamURL(base, w.SnapshotURL)
	if err != nil {
		return nil, err
	}
	return m.snapshotStream(ctx, u)
}

const snapshotBoundary = "printdisframe"

// snapshotStream turns a snapshot URL into an MJPEG stream of one frame a
// second. The first frame is fetched before returning, so a broken camera
// fails here rather than as an empty stream.
func (m *Monitor) snapshotStream(ctx context.Context, u string) (*CameraStream, error) {
	first, ct, err := m.snapshot(ctx, u)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go func() {
		frame, typ := first, ct
		for {
			if _, err := fmt.Fprintf(pw, "--%s\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n", snapshotBoundary, typ, len(frame)); err != nil {
				return
			}
			if _, err := pw.Write(append(frame, '\r', '\n')); err != nil {
				return
			}
			select {
			case <-ctx.Done():
				_ = pw.CloseWithError(ctx.Err())
				return
			case <-time.After(snapshotInterval):
			}
			f, t, err := m.snapshot(ctx, u)
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			frame, typ = f, t
		}
	}()
	return &CameraStream{ContentType: "multipart/x-mixed-replace; boundary=" + snapshotBoundary, Body: pr}, nil
}

func (m *Monitor) snapshot(ctx context.Context, u string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, streamHeaderTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	res, err := m.streamClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = res.Body.Close() }()
	ct := res.Header.Get("Content-Type")
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(ct, "image/") {
		return nil, "", fmt.Errorf("%s: status %d, content type %q", redact(u), res.StatusCode, ct)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxSnapshot))
	return b, ct, err
}

// ---- Bambuddy ----

type bambuddyToken struct {
	key     string // base URL and API key it was made for
	token   string
	expires time.Time
}

// bambuddyStreamToken returns a camera stream token, reusing one while it is
// fresh. Bambuddy wants it in the query string because browsers cannot send
// headers from an <img>; print-dis is the client here, but the rule stands.
func (m *Monitor) bambuddyStreamToken(ctx context.Context, base, apiKey string, fresh bool) (string, error) {
	cacheKey := base + "\x00" + apiKey
	m.tokenMu.Lock()
	defer m.tokenMu.Unlock()
	if !fresh && m.token.key == cacheKey && m.opts.Now().Before(m.token.expires) {
		return m.token.token, nil
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/printers/camera/stream-token", nil)
	if err != nil {
		return "", err
	}
	req.Header = bambuddyHeader(apiKey)
	res, err := m.opts.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bambuddy stream token: status %d", res.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&body); err != nil {
		return "", fmt.Errorf("bambuddy stream token: %w", err)
	}
	m.token = bambuddyToken{key: cacheKey, token: body.Token, expires: m.opts.Now().Add(tokenTTL)}
	return body.Token, nil
}

func (m *Monitor) openBambuddyCamera(ctx context.Context, t Target) (*CameraStream, error) {
	base := strings.TrimRight(t.BambuddyURL, "/")
	open := func(fresh bool) (*CameraStream, error) {
		q := url.Values{"fps": {"5"}}
		// With Bambuddy's sign-in off, minting a token can fail and none is needed.
		if tok, err := m.bambuddyStreamToken(ctx, base, t.BambuddyAPIKey, fresh); err == nil && tok != "" {
			q.Set("token", tok)
		}
		return m.openMJPEG(ctx, fmt.Sprintf("%s/api/v1/printers/%d/camera/stream?%s", base, t.BambuddyID, q.Encode()), nil)
	}
	s, err := open(false)
	if refused(err) {
		// The cached token may have been revoked; try once with a new one.
		s, err = open(true)
	}
	return s, err
}

// redact drops the query string, which may hold a token, from logged URLs.
func redact(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i] + "?…"
	}
	return u
}

// streamState is the Monitor's lazily built streaming client and token cache.
type streamState struct {
	streamOnce sync.Once
	stream     *http.Client
	tokenMu    sync.Mutex
	token      bambuddyToken
	listMu     sync.Mutex
	list       printerList
	camMu      sync.Mutex
	cams       map[int64]cameraCheck
}
