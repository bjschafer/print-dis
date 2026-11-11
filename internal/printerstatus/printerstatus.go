// Package printerstatus polls Moonraker and Bambuddy for what each printer is
// doing now. It is read-only: print-dis never starts, stops or queues a print.
package printerstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
)

const (
	fetchTimeout = 5 * time.Second
	pollInterval = 15 * time.Second
	// cameraCheckEvery is how often a Moonraker printer's webcam list is
	// re-read; cameras come and go far less often than jobs.
	cameraCheckEvery = 5 * time.Minute
	maxBody          = 4 << 20
)

// State is a printer's condition, common to both sources.
type State string

const (
	Unknown   State = "unknown" // not polled yet, or a reply print-dis does not understand
	Offline   State = "offline" // the source could not be reached, or the printer is not connected to it
	Idle      State = "idle"
	Printing  State = "printing" // includes preparing and heating
	Paused    State = "paused"
	Finished  State = "finished"  // the last job completed; the plate may need clearing
	Failed    State = "failed"    // the last job failed
	Cancelled State = "cancelled" // the last job was cancelled
)

// Label is the human name.
func (s State) Label() string {
	switch s {
	case Offline:
		return "Offline"
	case Idle:
		return "Idle"
	case Printing:
		return "Printing"
	case Paused:
		return "Paused"
	case Finished:
		return "Finished"
	case Failed:
		return "Failed"
	case Cancelled:
		return "Cancelled"
	}
	return "Unknown"
}

// Status is one reading of one printer.
type Status struct {
	State     State
	Job       string        // the file being, or last, printed; "" when none
	Progress  float64       // 0 to 1
	Remaining time.Duration // 0 when unknown
	Layer     int           // 0 when unknown
	Layers    int           // 0 when unknown
	Camera    bool          // a camera can be opened
	At        time.Time     // when this was read
}

// Active reports whether a job is on the printer now.
func (s Status) Active() bool { return s.State == Printing || s.State == Paused }

// Percent is Progress as a whole percentage.
func (s Status) Percent() int {
	p := int(s.Progress*100 + 0.5)
	return max(0, min(100, p))
}

// Target is one printer and where its status comes from: either
// MoonrakerURL, or BambuddyID with the Bambuddy it belongs to.
type Target struct {
	PrinterID    int64
	MoonrakerURL string

	BambuddyID     int
	BambuddyURL    string // Bambuddy's origin; "" makes the target read as unknown
	BambuddyAPIKey string // "" when Bambuddy's auth is off
}

// Options configure a Monitor.
type Options struct {
	// Targets lists what to poll; it is called at the start of every round so
	// that printer and settings edits take effect without a restart.
	Targets func(ctx context.Context) ([]Target, error)
	Log     *slog.Logger
	// Client, StreamClient, Now and Interval are for tests.
	Client       *http.Client
	StreamClient *http.Client
	Now          func() time.Time
	Interval     time.Duration
}

// Monitor polls every target in the background and keeps the latest reading
// of each. It is safe for concurrent use.
type Monitor struct {
	opts Options
	streamState

	mu       sync.RWMutex
	statuses map[int64]Status
}

// New returns a monitor; call Run to start polling.
func New(o Options) *Monitor {
	if o.Client == nil {
		o.Client = &http.Client{Timeout: fetchTimeout}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Interval <= 0 {
		o.Interval = pollInterval
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Monitor{opts: o, statuses: map[int64]Status{}}
}

// Status returns the latest reading for a printer, and false when there is
// none (no source configured, or not polled yet).
func (m *Monitor) Status(printerID int64) (Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.statuses[printerID]
	return s, ok
}

// Run polls until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.opts.Interval)
	defer t.Stop()
	for {
		m.Poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Poll reads every target once, concurrently, and records the results.
func (m *Monitor) Poll(ctx context.Context) {
	targets, err := m.opts.Targets(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.opts.Log.Warn("printer status: listing printers failed", "err", err)
		}
		return
	}
	results := make([]Status, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() { results[i] = m.read(ctx, t) })
	}
	wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.statuses
	m.statuses = make(map[int64]Status, len(targets))
	for i, t := range targets {
		m.statuses[t.PrinterID] = results[i]
		if prev, ok := old[t.PrinterID]; ok && prev.State != results[i].State {
			m.opts.Log.Info("printer status changed", "printer_id", t.PrinterID, "from", prev.State, "to", results[i].State)
		}
	}
}

func (m *Monitor) read(ctx context.Context, t Target) Status {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	var (
		s   Status
		err error
	)
	switch {
	case t.MoonrakerURL != "":
		s, err = m.moonraker(ctx, strings.TrimRight(t.MoonrakerURL, "/"))
		if err == nil {
			s.Camera = m.moonrakerHasCamera(ctx, t)
		}
	case t.BambuddyID > 0 && t.BambuddyURL != "":
		s, err = m.bambuddy(ctx, t)
		// Every Bambu printer has a camera; Bambuddy serves it while connected.
		s.Camera = err == nil && s.State != Offline
	default:
		s = Status{State: Unknown}
	}
	if err != nil {
		// Log only on the way down, so a printer that is off for the night
		// does not write a line every fifteen seconds.
		if prev, ok := m.Status(t.PrinterID); !ok || prev.State != Offline {
			m.opts.Log.Warn("printer status: read failed", "printer_id", t.PrinterID, "err", err)
		}
		s = Status{State: Offline}
	}
	s.At = m.opts.Now()
	return s
}

type cameraCheck struct {
	url string
	has bool
	at  time.Time
}

// moonrakerHasCamera reports whether Moonraker lists an enabled webcam,
// re-reading the list every few minutes. A failed read keeps the last answer.
func (m *Monitor) moonrakerHasCamera(ctx context.Context, t Target) bool {
	m.camMu.Lock()
	c, ok := m.cams[t.PrinterID]
	m.camMu.Unlock()
	if ok && c.url == t.MoonrakerURL && m.opts.Now().Sub(c.at) < cameraCheckEvery {
		return c.has
	}
	_, err := m.moonrakerWebcam(ctx, t.MoonrakerURL)
	has := err == nil
	if err != nil && !errors.Is(err, ErrNoCamera) && ok && c.url == t.MoonrakerURL {
		has = c.has
	}
	m.camMu.Lock()
	if m.cams == nil {
		m.cams = map[int64]cameraCheck{}
	}
	m.cams[t.PrinterID] = cameraCheck{url: t.MoonrakerURL, has: has, at: m.opts.Now()}
	m.camMu.Unlock()
	return has
}

func (m *Monitor) getJSON(ctx context.Context, url string, header http.Header, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header = header
	req.Header.Set("Accept", "application/json")
	res, err := m.opts.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", url, res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(v); err != nil {
		return fmt.Errorf("%s: bad response: %w", url, err)
	}
	return nil
}

// ---- Moonraker ----

type moonrakerReply struct {
	Result *struct {
		Status struct {
			PrintStats struct {
				State         string  `json:"state"`
				Filename      string  `json:"filename"`
				PrintDuration float64 `json:"print_duration"`
				Info          struct {
					CurrentLayer *int `json:"current_layer"`
					TotalLayer   *int `json:"total_layer"`
				} `json:"info"`
			} `json:"print_stats"`
			VirtualSDCard struct {
				Progress float64 `json:"progress"`
			} `json:"virtual_sdcard"`
			DisplayStatus struct {
				Progress float64 `json:"progress"`
			} `json:"display_status"`
		} `json:"status"`
	} `json:"result"`
}

func (m *Monitor) moonraker(ctx context.Context, base string) (Status, error) {
	var r moonrakerReply
	if err := m.getJSON(ctx, base+"/printer/objects/query?print_stats&virtual_sdcard&display_status", http.Header{}, &r); err != nil {
		return Status{}, err
	}
	if r.Result == nil {
		return Status{}, errors.New("moonraker: reply has no result")
	}
	st := r.Result.Status
	ps := st.PrintStats
	s := Status{Job: jobName(ps.Filename)}
	switch ps.State {
	case "standby":
		s.State, s.Job = Idle, ""
	case "printing":
		s.State = Printing
	case "paused":
		s.State = Paused
	case "complete":
		s.State = Finished
	case "cancelled":
		s.State = Cancelled
	case "error":
		s.State = Failed
	default:
		s.State = Unknown
	}
	// display_status follows M73 from the slicer when it is sent, which is
	// closer to the truth than the file position.
	s.Progress = st.DisplayStatus.Progress
	if s.Progress <= 0 {
		s.Progress = st.VirtualSDCard.Progress
	}
	if s.State == Finished {
		s.Progress = 1
	}
	if s.Active() && s.Progress >= 0.01 && ps.PrintDuration > 0 {
		total := ps.PrintDuration / s.Progress
		s.Remaining = time.Duration(total-ps.PrintDuration) * time.Second
	}
	if ps.Info.CurrentLayer != nil && ps.Info.TotalLayer != nil && *ps.Info.TotalLayer > 0 {
		s.Layer, s.Layers = *ps.Info.CurrentLayer, *ps.Info.TotalLayer
	}
	return s, nil
}

// ---- Bambuddy ----

type bambuddyReply struct {
	Connected     bool     `json:"connected"`
	State         *string  `json:"state"`
	SubtaskName   *string  `json:"subtask_name"`
	GcodeFile     *string  `json:"gcode_file"`
	Progress      *float64 `json:"progress"`
	RemainingTime *int     `json:"remaining_time"` // minutes
	LayerNum      *int     `json:"layer_num"`
	TotalLayers   *int     `json:"total_layers"`
}

func (m *Monitor) bambuddy(ctx context.Context, t Target) (Status, error) {
	h := bambuddyHeader(t.BambuddyAPIKey)
	var r bambuddyReply
	base := strings.TrimRight(t.BambuddyURL, "/")
	if err := m.getJSON(ctx, fmt.Sprintf("%s/api/v1/printers/%d/status", base, t.BambuddyID), h, &r); err != nil {
		return Status{}, err
	}
	if !r.Connected {
		return Status{State: Offline}, nil
	}
	s := Status{State: Unknown}
	switch deref(r.State) {
	case "IDLE":
		s.State = Idle
	case "PREPARE", "SLICING", "RUNNING":
		s.State = Printing
	case "PAUSE":
		s.State = Paused
	case "FINISH":
		s.State = Finished
	case "FAILED":
		s.State = Failed
	}
	if s.State != Idle {
		s.Job = strings.TrimSpace(deref(r.SubtaskName))
		if s.Job == "" {
			s.Job = jobName(deref(r.GcodeFile))
		}
	}
	if r.Progress != nil {
		s.Progress = max(0, min(1, *r.Progress/100))
	}
	if s.State == Finished {
		s.Progress = 1
	}
	if s.Active() && r.RemainingTime != nil && *r.RemainingTime > 0 {
		s.Remaining = time.Duration(*r.RemainingTime) * time.Minute
	}
	if r.LayerNum != nil && r.TotalLayers != nil && *r.TotalLayers > 0 {
		s.Layer, s.Layers = *r.LayerNum, *r.TotalLayers
	}
	return s, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// jobName turns "dir/Benchy_PLA_1h2m.gcode" into "Benchy_PLA_1h2m".
func jobName(file string) string {
	name := path.Base(strings.TrimSpace(file))
	if name == "." || name == "/" {
		return ""
	}
	for _, ext := range []string{".gcode", ".3mf"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			name = name[:len(name)-len(ext)]
		}
	}
	return name
}

// TargetFor is where p's status comes from under the given settings, and
// false when p has no source.
func TargetFor(p *core.Printer, set core.Settings) (Target, bool) {
	switch {
	case p.MoonrakerURL != "":
		return Target{PrinterID: p.ID, MoonrakerURL: p.MoonrakerURL}, true
	case p.BambuddyPrinterID != nil:
		return Target{PrinterID: p.ID, BambuddyID: *p.BambuddyPrinterID, BambuddyURL: set.BambuddyURL, BambuddyAPIKey: set.BambuddyAPIKey}, true
	}
	return Target{}, false
}
