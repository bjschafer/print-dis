package printerstatus

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mjpeg writes n frames of an MJPEG stream, then holds until the client goes.
func mjpeg(n int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=b")
		for i := range n {
			_, _ = fmt.Fprintf(w, "--b\r\nContent-Type: image/jpeg\r\n\r\nframe%d\r\n", i)
		}
		// A part ends at the next boundary, so the last frame needs one too.
		_, _ = io.WriteString(w, "--b\r\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
}

// readFrames reads up to n parts from a camera stream.
func readFrames(t *testing.T, s *CameraStream, n int) []string {
	t.Helper()
	_, params, err := mime.ParseMediaType(s.ContentType)
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(bufio.NewReader(s.Body), params["boundary"])
	var out []string
	for range n {
		p, err := mr.NextPart()
		if err != nil {
			t.Fatalf("after %d frames: %v", len(out), err)
		}
		b, _ := io.ReadAll(p)
		out = append(out, string(b))
	}
	return out
}

func webcams(list string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server/webcams/list" {
			http.NotFound(w, r)
			return
		}
		json200(`{"result":{"webcams":`+list+`}}`)(w, r)
	}
}

func TestMoonrakerCameraMJPEG(t *testing.T) {
	cam := serve(t, mjpeg(2))
	mr := serve(t, webcams(`[
		{"name":"off","enabled":false,"service":"mjpegstreamer","stream_url":"http://nowhere.invalid/"},
		{"name":"Bed","enabled":true,"service":"mjpegstreamer","stream_url":"`+cam.URL+`/?action=stream","snapshot_url":"","rotation":90,"flip_horizontal":true}]`))
	m := monitor(nil, Options{})
	tg := Target{PrinterID: 1, MoonrakerURL: mr.URL}

	info, err := m.Camera(context.Background(), tg)
	if err != nil || info != (CameraInfo{Name: "Bed", Rotation: 90, FlipH: true}) {
		t.Fatalf("info %+v %v", info, err)
	}
	s, err := m.OpenCamera(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Body.Close() }()
	if got := readFrames(t, s, 2); got[0] != "frame0" || got[1] != "frame1" {
		t.Errorf("frames %q", got)
	}
}

func TestMoonrakerCameraFromSnapshots(t *testing.T) {
	var shots atomic.Int32
	cam := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = fmt.Fprintf(w, "shot%d", shots.Add(1))
	})
	// camera-streamer's WebRTC address cannot be proxied; its snapshot can.
	mr := serve(t, webcams(`[{"name":"cam","service":"webrtc-camerastreamer","stream_url":"/webcam/webrtc","snapshot_url":"`+cam.URL+`/snapshot"}]`))
	m := monitor(nil, Options{})
	s, err := m.OpenCamera(context.Background(), Target{PrinterID: 1, MoonrakerURL: mr.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Body.Close() }()
	if got := readFrames(t, s, 2); got[0] != "shot1" || got[1] != "shot2" {
		t.Errorf("frames %q", got)
	}
}

func TestMoonrakerCameraFailures(t *testing.T) {
	broken := serve(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusBadGateway) })
	for name, list := range map[string]string{
		"none":            `[]`,
		"all disabled":    `[{"name":"x","enabled":false,"stream_url":"/s"}]`,
		"broken snapshot": `[{"name":"x","service":"webrtc-camerastreamer","snapshot_url":"` + broken.URL + `"}]`,
		"broken stream":   `[{"name":"x","service":"mjpegstreamer","stream_url":"` + broken.URL + `"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			mr := serve(t, webcams(list))
			m := monitor(nil, Options{})
			if s, err := m.OpenCamera(context.Background(), Target{PrinterID: 1, MoonrakerURL: mr.URL}); err == nil {
				_ = s.Body.Close()
				t.Error("opened a camera that does not work")
			}
		})
	}
	m := monitor(nil, Options{})
	if _, err := m.OpenCamera(context.Background(), Target{PrinterID: 1}); !errors.Is(err, ErrNoCamera) {
		t.Errorf("no source: %v", err)
	}
	if _, err := m.Camera(context.Background(), Target{PrinterID: 1, MoonrakerURL: "http://127.0.0.1:1"}); err == nil {
		t.Error("unreachable Moonraker gave camera info")
	}
}

func TestWebcamURL(t *testing.T) {
	for _, tt := range []struct{ base, ref, want string }{
		{"http://trident.lan:7125", "/webcam/?action=stream", "http://trident.lan/webcam/?action=stream"},
		{"http://trident.lan", "webcam/?action=snapshot", "http://trident.lan/webcam/?action=snapshot"},
		{"https://10.0.0.5:7125/", "http://10.0.0.6:8080/?action=stream", "http://10.0.0.6:8080/?action=stream"},
		{"http://[fd00::5]:7125", "/webcam/", "http://[fd00::5]/webcam/"},
	} {
		if got, err := webcamURL(tt.base, tt.ref); err != nil || got != tt.want {
			t.Errorf("webcamURL(%q, %q) = %q, %v; want %q", tt.base, tt.ref, got, err, tt.want)
		}
	}
	for _, ref := range []string{"", "rtsp://cam/stream"} {
		if _, err := webcamURL("http://x:7125", ref); err == nil {
			t.Errorf("webcamURL accepted %q", ref)
		}
	}
}

func TestBambuddyCamera(t *testing.T) {
	var mints, streams atomic.Int32
	var gotToken, gotKey atomic.Value
	bb := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/printers/camera/stream-token":
			gotKey.Store(r.Header.Get("X-API-Key"))
			json200(fmt.Sprintf(`{"token":"tok%d"}`, mints.Add(1)))(w, r)
		case r.URL.Path == "/api/v1/printers/3/camera/stream":
			streams.Add(1)
			gotToken.Store(r.URL.Query().Get("token"))
			mjpeg(1)(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	m := monitor(nil, Options{})
	tg := Target{PrinterID: 1, BambuddyID: 3, BambuddyURL: bb.URL + "/", BambuddyAPIKey: "bb_key"}
	if _, err := m.Camera(context.Background(), tg); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		s, err := m.OpenCamera(context.Background(), tg)
		if err != nil {
			t.Fatal(err)
		}
		if got := readFrames(t, s, 1); got[0] != "frame0" {
			t.Errorf("frames %q", got)
		}
		_ = s.Body.Close()
	}
	if mints.Load() != 1 || gotToken.Load() != "tok1" || gotKey.Load() != "bb_key" {
		t.Errorf("token should be minted once with the key and reused: mints %d token %v key %v", mints.Load(), gotToken.Load(), gotKey.Load())
	}
}

func TestBambuddyCameraRetriesWithFreshToken(t *testing.T) {
	var mints atomic.Int32
	bb := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/printers/camera/stream-token":
			json200(fmt.Sprintf(`{"token":"tok%d"}`, mints.Add(1)))(w, r)
		case "/api/v1/printers/3/camera/stream":
			if r.URL.Query().Get("token") == "tok1" { // revoked
				http.Error(w, "bad token", http.StatusUnauthorized)
				return
			}
			mjpeg(1)(w, r)
		}
	})
	m := monitor(nil, Options{})
	s, err := m.OpenCamera(context.Background(), Target{PrinterID: 1, BambuddyID: 3, BambuddyURL: bb.URL})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Body.Close()
	if mints.Load() != 2 {
		t.Errorf("mints %d, want 2", mints.Load())
	}
}

func TestBambuddyCameraWithoutAuthAndFailures(t *testing.T) {
	// Sign-in off: minting fails, the stream opens without a token.
	open := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream-token") {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Has("token") {
			t.Error("sent a token it does not have")
		}
		mjpeg(1)(w, r)
	})
	m := monitor(nil, Options{})
	s, err := m.OpenCamera(context.Background(), Target{PrinterID: 1, BambuddyID: 3, BambuddyURL: open.URL})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Body.Close()

	// A camera that answers with an error page is an error, not a stream.
	notCam := serve(t, json200(`{"detail":"camera offline"}`))
	if s, err := m.OpenCamera(context.Background(), Target{PrinterID: 1, BambuddyID: 3, BambuddyURL: notCam.URL}); err == nil {
		_ = s.Body.Close()
		t.Error("opened a non-MJPEG reply")
	}
}

func TestCameraSlowToStartTimesOut(t *testing.T) {
	slow := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream-token") {
			json200(`{"token":"t"}`)(w, r)
			return
		}
		<-r.Context().Done()
	})
	m := monitor(nil, Options{StreamClient: &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 100 * time.Millisecond}}})
	start := time.Now()
	_, err := m.OpenCamera(context.Background(), Target{PrinterID: 1, BambuddyID: 3, BambuddyURL: slow.URL})
	if err == nil || time.Since(start) > 5*time.Second {
		t.Errorf("err %v after %v", err, time.Since(start))
	}
}

func TestBambuddyPrinters(t *testing.T) {
	var key string
	bb := serve(t, func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("X-API-Key")
		if r.URL.Path != "/api/v1/printers/" {
			http.NotFound(w, r)
			return
		}
		json200(`[{"id":1,"name":"H2D","model":"H2D","serial_number":"x"},{"id":4,"name":"Spare","model":"P1S"}]`)(w, r)
	})
	m := monitor(nil, Options{})
	got, err := m.BambuddyPrinters(context.Background(), bb.URL+"/", "bb_k")
	if err != nil || len(got) != 2 || got[0] != (BambuddyPrinter{1, "H2D", "H2D"}) || got[1].ID != 4 || key != "bb_k" {
		t.Errorf("got %+v %v key %q", got, err, key)
	}
	if _, err := m.BambuddyPrinters(context.Background(), "http://127.0.0.1:1", ""); err == nil {
		t.Error("unreachable Bambuddy listed printers")
	}
}
