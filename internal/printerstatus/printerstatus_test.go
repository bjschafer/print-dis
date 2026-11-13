package printerstatus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

func monitor(targets []Target, o Options) *Monitor {
	o.Targets = func(context.Context) ([]Target, error) { return targets, nil }
	o.Now = func() time.Time { return t0 }
	o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(o)
}

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func json200(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func TestMoonraker(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Status
	}{
		{"printing", `{"result":{"eventtime":1,"status":{
			"print_stats":{"state":"printing","filename":"jobs/Benchy_PLA.gcode","print_duration":600,"info":{"current_layer":12,"total_layer":48}},
			"virtual_sdcard":{"progress":0.2},"display_status":{"progress":0.25}}}}`,
			Status{State: Printing, Job: "Benchy_PLA", Progress: 0.25, Remaining: 30 * time.Minute, Layer: 12, Layers: 48, At: t0}},
		{"no M73 falls back to file position", `{"result":{"status":{
			"print_stats":{"state":"paused","filename":"a.gcode","print_duration":100,"info":{"current_layer":null,"total_layer":null}},
			"virtual_sdcard":{"progress":0.5},"display_status":{"progress":0}}}}`,
			Status{State: Paused, Job: "a", Progress: 0.5, Remaining: 100 * time.Second, At: t0}},
		{"standby clears the job", `{"result":{"status":{"print_stats":{"state":"standby","filename":"old.gcode"},"virtual_sdcard":{"progress":0}}}}`,
			Status{State: Idle, At: t0}},
		{"complete", `{"result":{"status":{"print_stats":{"state":"complete","filename":"done.gcode","print_duration":900},"virtual_sdcard":{"progress":0.99}}}}`,
			Status{State: Finished, Job: "done", Progress: 1, At: t0}},
		{"cancelled", `{"result":{"status":{"print_stats":{"state":"cancelled","filename":"x.gcode"}}}}`,
			Status{State: Cancelled, Job: "x", At: t0}},
		{"error", `{"result":{"status":{"print_stats":{"state":"error","filename":"x.gcode"}}}}`,
			Status{State: Failed, Job: "x", At: t0}},
		{"unrecognised", `{"result":{"status":{"print_stats":{"state":"levitating"}}}}`,
			Status{State: Unknown, At: t0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/server/webcams/list" {
					json200(`{"result":{"webcams":[]}}`)(w, r)
					return
				}
				path = r.URL.RequestURI()
				json200(tt.body)(w, r)
			})
			m := monitor([]Target{{PrinterID: 7, MoonrakerURL: srv.URL + "/"}}, Options{})
			m.Poll(context.Background())
			got, ok := m.Status(7)
			if !ok || got != tt.want {
				t.Errorf("got %+v %v, want %+v", got, ok, tt.want)
			}
			if path != "/printer/objects/query?print_stats&virtual_sdcard&display_status" {
				t.Errorf("path %q", path)
			}
		})
	}
}

func TestBambuddy(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Status
	}{
		{"running", `{"id":3,"name":"H2D","connected":true,"state":"RUNNING","subtask_name":"Gridfinity bins","gcode_file":"/data/Metadata/plate_1.gcode","progress":42,"remaining_time":75,"layer_num":30,"total_layers":120,"ams":[]}`,
			Status{State: Printing, Job: "Gridfinity bins", Progress: 0.42, Remaining: 75 * time.Minute, Layer: 30, Layers: 120, Camera: true, At: t0}},
		{"preparing uses gcode name without subtask", `{"connected":true,"state":"PREPARE","subtask_name":null,"gcode_file":"/data/Metadata/plate_2.gcode","progress":0,"remaining_time":0}`,
			Status{State: Printing, Job: "plate_2", Camera: true, At: t0}},
		{"paused", `{"connected":true,"state":"PAUSE","subtask_name":"Lid","progress":80.4,"remaining_time":10}`,
			Status{State: Paused, Job: "Lid", Progress: 0.804, Remaining: 10 * time.Minute, Camera: true, At: t0}},
		{"finished", `{"connected":true,"state":"FINISH","subtask_name":"Lid","progress":100,"remaining_time":0}`,
			Status{State: Finished, Job: "Lid", Progress: 1, Camera: true, At: t0}},
		{"failed", `{"connected":true,"state":"FAILED","subtask_name":"Lid","progress":12}`,
			Status{State: Failed, Job: "Lid", Progress: 0.12, Camera: true, At: t0}},
		{"idle hides the old job", `{"connected":true,"state":"IDLE","subtask_name":"Lid","progress":0}`,
			Status{State: Idle, Camera: true, At: t0}},
		{"not connected to Bambuddy", `{"id":3,"name":"H2D","connected":false}`,
			Status{State: Offline, At: t0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path, key string
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				path, key = r.URL.Path, r.Header.Get("X-API-Key")
				json200(tt.body)(w, r)
			})
			m := monitor([]Target{{PrinterID: 1, BambuddyID: 3, BambuddyURL: srv.URL + "/", BambuddyAPIKey: "bb_test"}}, Options{})
			m.Poll(context.Background())
			got, _ := m.Status(1)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if path != "/api/v1/printers/3/status" || key != "bb_test" {
				t.Errorf("path %q key %q", path, key)
			}
		})
	}
}

func TestFailuresReadOffline(t *testing.T) {
	slow := serve(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	broken := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "klippy not ready", http.StatusServiceUnavailable)
	})
	garbage := serve(t, json200(`<html>`))
	unauthorised := serve(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusUnauthorized) })
	m := monitor([]Target{
		{PrinterID: 1, MoonrakerURL: slow.URL},
		{PrinterID: 2, MoonrakerURL: broken.URL},
		{PrinterID: 3, MoonrakerURL: garbage.URL},
		{PrinterID: 4, MoonrakerURL: "http://127.0.0.1:1"}, // refused
		{PrinterID: 5, BambuddyID: 9, BambuddyURL: unauthorised.URL},
	}, Options{Client: &http.Client{Timeout: 200 * time.Millisecond}})

	start := time.Now()
	m.Poll(context.Background())
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("poll took %v; targets should be read concurrently with a timeout", d)
	}
	for id := int64(1); id <= 5; id++ {
		if s, _ := m.Status(id); s.State != Offline {
			t.Errorf("printer %d: %v, want offline", id, s.State)
		}
	}
}

func TestBambuddyTargetWithoutURLIsUnknown(t *testing.T) {
	m := monitor([]Target{{PrinterID: 1, BambuddyID: 3}}, Options{})
	m.Poll(context.Background())
	if s, ok := m.Status(1); !ok || s.State != Unknown {
		t.Errorf("got %+v %v", s, ok)
	}
}

func TestPollFollowsTargets(t *testing.T) {
	srv := serve(t, json200(`{"result":{"status":{"print_stats":{"state":"standby"}}}}`))
	var targets atomic.Value
	targets.Store([]Target{{PrinterID: 1, MoonrakerURL: srv.URL}})
	var fail atomic.Bool
	m := New(Options{
		Targets: func(context.Context) ([]Target, error) {
			if fail.Load() {
				return nil, errors.New("db down")
			}
			return targets.Load().([]Target), nil
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if _, ok := m.Status(1); ok {
		t.Fatal("status before the first poll")
	}
	m.Poll(context.Background())
	if s, ok := m.Status(1); !ok || s.State != Idle {
		t.Fatalf("got %+v %v", s, ok)
	}
	// A failed listing keeps the last readings.
	fail.Store(true)
	m.Poll(context.Background())
	if _, ok := m.Status(1); !ok {
		t.Error("lost status after a failed listing")
	}
	// A printer whose source is removed drops out.
	fail.Store(false)
	targets.Store([]Target{})
	m.Poll(context.Background())
	if _, ok := m.Status(1); ok {
		t.Error("status kept for a removed target")
	}
}

func TestSinceAndClearPlate(t *testing.T) {
	var reply atomic.Value // "" makes the printer unreachable
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/server/webcams/list" {
			json200(`{"result":{"webcams":[]}}`)(w, r)
			return
		}
		state := reply.Load().(string)
		if state == "" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		json200(`{"result":{"status":{"print_stats":{"state":"`+state+`","filename":"lid.gcode"}}}}`)(w, r)
	})
	now := t0
	m := New(Options{
		Targets: func(context.Context) ([]Target, error) { return []Target{{PrinterID: 1, MoonrakerURL: srv.URL}}, nil },
		Now:     func() time.Time { return now },
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	step := func(state string) Status {
		t.Helper()
		now = now.Add(time.Minute)
		reply.Store(state)
		m.Poll(context.Background())
		s, _ := m.Status(1)
		return s
	}

	// Already finished at the first reading: when is unknown.
	if s := step("complete"); s.State != Finished || !s.Since.IsZero() {
		t.Fatalf("first reading: %+v", s)
	}
	// Clearing the plate reads idle, and stays so while the printer still
	// says finished.
	m.ClearPlate(1)
	if s, _ := m.Status(1); s.State != Idle || s.Job != "" {
		t.Fatalf("after clearing: %+v", s)
	}
	if s := step("complete"); s.State != Idle {
		t.Fatalf("cleared plate came back: %+v", s)
	}
	// A watched job: printing, then finished at a known time.
	if s := step("printing"); s.State != Printing || !s.Since.Equal(now) {
		t.Fatalf("printing: %+v", s)
	}
	m.ClearPlate(1) // nothing to clear mid-job
	if s, _ := m.Status(1); s.State != Printing {
		t.Fatalf("cleared while printing: %+v", s)
	}
	finishedAt := now.Add(time.Minute)
	if s := step("complete"); s.State != Finished || !s.Since.Equal(finishedAt) {
		t.Fatalf("finished: %+v, want since %v", s, finishedAt)
	}
	// Dropping offline and back keeps when it finished.
	if s := step(""); s.State != Offline {
		t.Fatalf("down: %+v", s)
	}
	if s := step("complete"); s.State != Finished || !s.Since.Equal(finishedAt) {
		t.Fatalf("back after a blip: %+v, want since %v", s, finishedAt)
	}
	// A job that ends while the printer is offline has no time.
	step("printing")
	step("")
	if s := step("complete"); s.State != Finished || !s.Since.IsZero() {
		t.Fatalf("finished while offline: %+v", s)
	}
}

func TestRunStopsWithContext(t *testing.T) {
	var n atomic.Int32
	m := New(Options{
		Targets:  func(context.Context) ([]Target, error) { n.Add(1); return nil, nil },
		Interval: 5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	for n.Load() < 2 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestPercentAndJobName(t *testing.T) {
	for _, tt := range []struct {
		p    float64
		want int
	}{{0, 0}, {0.424, 42}, {0.996, 100}, {1.5, 100}, {-1, 0}} {
		if got := (Status{Progress: tt.p}).Percent(); got != tt.want {
			t.Errorf("Percent(%v) = %d", tt.p, got)
		}
	}
	for in, want := range map[string]string{"": "", "a/b/Cube.GCODE": "Cube", "plate.3mf": "plate", "notes.txt": "notes.txt", "/": ""} {
		if got := jobName(in); got != want {
			t.Errorf("jobName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMoonrakerCameraCheck(t *testing.T) {
	var lists atomic.Int32
	var webcam atomic.Value
	webcam.Store(`[{"name":"Default","enabled":true,"stream_url":"/webcam/?action=stream"}]`)
	var down atomic.Bool
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/server/webcams/list" {
			lists.Add(1)
			if down.Load() {
				http.Error(w, "no", http.StatusServiceUnavailable)
				return
			}
			json200(`{"result":{"webcams":`+webcam.Load().(string)+`}}`)(w, r)
			return
		}
		json200(`{"result":{"status":{"print_stats":{"state":"standby"}}}}`)(w, r)
	})
	now := t0
	m := New(Options{
		Targets: func(context.Context) ([]Target, error) { return []Target{{PrinterID: 1, MoonrakerURL: srv.URL}}, nil },
		Now:     func() time.Time { return now },
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	poll := func() bool {
		m.Poll(context.Background())
		s, _ := m.Status(1)
		return s.Camera
	}
	first := poll()
	second := poll()
	if !first || !second || lists.Load() != 1 {
		t.Fatalf("camera should be found once and cached: %v %v, %d list reads", first, second, lists.Load())
	}
	// Removed in Moonraker: gone after the next check.
	webcam.Store(`[]`)
	now = now.Add(cameraCheckEvery)
	if poll() {
		t.Error("camera still shown after it was removed")
	}
	// A failed check keeps the last answer.
	webcam.Store(`[{"name":"Default","stream_url":"/s"}]`)
	now = now.Add(cameraCheckEvery)
	if !poll() {
		t.Fatal("camera not found again")
	}
	down.Store(true)
	now = now.Add(cameraCheckEvery)
	if !poll() {
		t.Error("a failed check hid the camera")
	}
}
