package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func pngBytes(t testing.TB) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func page(head string) string {
	return "<!doctype html><html><head>" + head + "</head><body>hi</body></html>"
}

func serve(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func local() *Fetcher { return New(WithAllowLoopback()) }

func TestFetchTitles(t *testing.T) {
	img := pngBytes(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/og", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page(`<title>Plain</title><meta property="og:title" content="  Headphone   stand "><meta property="og:image" content="/img.png">`)))
	})
	mux.HandleFunc("/title", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page(`<title>
  Just a title </title>`)))
	})
	mux.HandleFunc("/img.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain") // wrong on purpose: bytes are sniffed
		_, _ = w.Write(img)
	})
	s := serve(t, mux)

	p, err := local().Fetch(context.Background(), s.URL+"/og")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Headphone stand" {
		t.Errorf("title = %q", p.Title)
	}
	if p.ImageType != "image/png" || !bytes.Equal(p.Image, img) {
		t.Errorf("relative og:image not fetched: type %q, %d bytes", p.ImageType, len(p.Image))
	}

	p, err = local().Fetch(context.Background(), s.URL+"/title")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Just a title" || p.Image != nil {
		t.Errorf("fallback: %+v", p)
	}
}

func TestFetchSendsBrowserHeaders(t *testing.T) {
	var ua, accept string
	s := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, accept = r.UserAgent(), r.Header.Get("Accept")
		_, _ = w.Write([]byte(page("<title>x</title>")))
	}))
	if _, err := local().Fetch(context.Background(), s.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ua, "Mozilla/5.0") || !strings.Contains(accept, "text/html") {
		t.Errorf("ua %q accept %q", ua, accept)
	}
}

func TestImageDropped(t *testing.T) {
	big := append(pngBytes(t), make([]byte, MaxImageBytes)...)
	mux := http.NewServeMux()
	mux.HandleFunc("/big.png", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(big) })
	mux.HandleFunc("/chunked.png", func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Length: the cap must hold while reading.
		w.(http.Flusher).Flush()
		_, _ = w.Write(big)
	})
	mux.HandleFunc("/text.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png") // lies
		_, _ = w.Write([]byte("<html>not an image</html>"))
	})
	mux.HandleFunc("/svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	})
	mux.HandleFunc("/gone.png", http.NotFound)
	s := serve(t, mux)
	for _, path := range []string{"/big.png", "/chunked.png", "/text.png", "/svg", "/gone.png"} {
		mux.HandleFunc("/page"+path, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, page(`<meta property="og:title" content="T"><meta property="og:image" content="`+s.URL+path+`">`))
		})
		p, err := local().Fetch(context.Background(), s.URL+"/page"+path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if p.Title != "T" || p.Image != nil || p.ImageType != "" {
			t.Errorf("%s: image should be dropped, got %d bytes %q", path, len(p.Image), p.ImageType)
		}
	}
}

func TestHTMLReadCap(t *testing.T) {
	pad := strings.Repeat("<!-- pad -->", MaxHTMLBytes/12+10)
	s := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/early" {
			_, _ = w.Write([]byte(page(`<title>Early</title>` + pad)))
			return
		}
		_, _ = w.Write([]byte(page(pad + `<title>Late</title>`)))
	}))
	p, err := local().Fetch(context.Background(), s.URL+"/early")
	if err != nil || p.Title != "Early" {
		t.Errorf("early: %+v %v", p, err)
	}
	if _, err := local().Fetch(context.Background(), s.URL+"/late"); !errors.Is(err, ErrNothing) {
		t.Errorf("title past the cap should not be read: %v", err)
	}
}

func TestFetchFailures(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/403", func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "Just a moment...", http.StatusForbidden)
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"x"}`))
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page(""))) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	})
	s := serve(t, mux)
	for _, path := range []string{"/403", "/json", "/empty", "/nope"} {
		if p, err := local().Fetch(context.Background(), s.URL+path); err == nil {
			t.Errorf("%s: want error, got %+v", path, p)
		}
	}
	start := time.Now()
	_, err := New(WithAllowLoopback(), WithTimeout(150*time.Millisecond)).Fetch(context.Background(), s.URL+"/slow")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("slow server: err %v after %v", err, time.Since(start))
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := local().Fetch(cctx, s.URL+"/403"); err == nil {
		t.Error("cancelled context should fail")
	}
}

func TestRedirectLimit(t *testing.T) {
	s := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte(page("<title>Arrived</title>")))
			return
		}
		var n int
		_, _ = fmt.Sscanf(r.URL.Path, "/hop/%d", &n)
		if n == 0 {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n-1), http.StatusFound)
	}))
	if p, err := local().Fetch(context.Background(), s.URL+"/hop/3"); err != nil || p.Title != "Arrived" {
		t.Errorf("4 redirects should work: %+v %v", p, err)
	}
	if _, err := local().Fetch(context.Background(), s.URL+"/hop/9"); err == nil {
		t.Error("10 redirects should fail")
	}
}

func TestRefusesNonHTTP(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com/x", "gopher://example.com", "javascript:alert(1)", "//example.com", "http://", ""} {
		if _, err := New().Fetch(context.Background(), raw); err == nil {
			t.Errorf("%q should be refused", raw)
		}
	}
}

func TestRedirectToOtherScheme(t *testing.T) {
	s := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	if _, err := local().Fetch(context.Background(), s.URL); err == nil {
		t.Error("redirect to file: should fail")
	}
}

// ---- SSRF guard ----

func okServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	return serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(page("<title>Internal</title>")))
	}))
}

func TestStrictDefaultsRefuseLoopback(t *testing.T) {
	var hits atomic.Int32
	s := okServer(t, &hits)
	u, _ := url.Parse(s.URL)
	port := u.Port()
	for _, host := range []string{"127.0.0.1", "localhost", "[::1]", "[::ffff:127.0.0.1]", "0.0.0.0", "[::]"} {
		_, err := New().Fetch(context.Background(), "http://"+host+":"+port+"/")
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: want ErrBlocked, got %v", host, err)
		}
	}
	for _, host := range []string{"10.0.0.1", "192.168.1.1", "172.16.0.1", "169.254.169.254", "100.64.0.1", "[fd00::1]", "[fe80::1]", "[::ffff:10.0.0.1]", "224.0.0.1", "[64:ff9b::7f00:1]"} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := New().Fetch(ctx, "http://"+host+"/")
		cancel()
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: want ErrBlocked, got %v", host, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("server was reached %d times", hits.Load())
	}
}

func TestAllowlistedHostMayBePrivate(t *testing.T) {
	var hits atomic.Int32
	s := okServer(t, &hits)
	u, _ := url.Parse(s.URL)

	p, err := New(WithAllowHosts("127.0.0.1")).Fetch(context.Background(), s.URL)
	if err != nil || p.Title != "Internal" {
		t.Fatalf("allowlisted: %+v %v", p, err)
	}
	// Allowlisting one name does not open its siblings.
	if _, err := New(WithAllowHosts("127.0.0.1")).Fetch(context.Background(), "http://localhost:"+u.Port()); !errors.Is(err, ErrBlocked) {
		t.Errorf("localhost not allowlisted: %v", err)
	}
}

func TestAllowedHostRedirectingToRefusedHost(t *testing.T) {
	var hits atomic.Int32
	target := okServer(t, &hits)
	tu, _ := url.Parse(target.URL)
	front := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Same machine, different name: not on the allowlist.
		http.Redirect(w, r, "http://localhost:"+tu.Port()+"/", http.StatusFound)
	}))
	f := New(WithAllowHosts("127.0.0.1"))
	if _, err := f.Fetch(context.Background(), front.URL); !errors.Is(err, ErrBlocked) {
		t.Errorf("redirect to non-allowlisted private host: %v", err)
	}
	if hits.Load() != 0 {
		t.Error("redirect target was reached")
	}

	// Redirect to a private literal IP is likewise refused.
	front2 := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	if _, err := f.Fetch(context.Background(), front2.URL); !errors.Is(err, ErrBlocked) {
		t.Errorf("redirect to link-local: %v", err)
	}

	// And an allowlisted target is fine.
	front3 := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	if p, err := f.Fetch(context.Background(), front3.URL); err != nil || p.Title != "Internal" {
		t.Errorf("allowlisted redirect target: %+v %v", p, err)
	}
}

func TestImageDownloadUsesGuard(t *testing.T) {
	var hits atomic.Int32
	imgSrv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(pngBytes(t))
	}))
	iu, _ := url.Parse(imgSrv.URL)
	// The page is allowed (127.0.0.1) but names its image by another host.
	pageSrv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, page(`<meta property="og:title" content="T"><meta property="og:image" content="http://localhost:`+iu.Port()+`/i.png">`))
	}))
	p, err := New(WithAllowHosts("127.0.0.1")).Fetch(context.Background(), pageSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.Image != nil || hits.Load() != 0 {
		t.Errorf("image from a refused host was fetched (%d hits)", hits.Load())
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		ip   string
		want bool // true = refused
	}{
		{"127.0.0.1", true}, {"127.1.2.3", true}, {"::1", true},
		{"10.1.2.3", true}, {"172.16.0.1", true}, {"172.31.255.255", true}, {"192.168.0.1", true},
		{"169.254.169.254", true}, {"fe80::1", true}, {"fd12:3456::1", true}, {"fc00::1", true},
		{"0.0.0.0", true}, {"::", true}, {"0.1.2.3", true},
		{"224.0.0.1", true}, {"ff02::1", true}, {"255.255.255.255", true},
		{"100.64.0.1", true}, {"100.127.255.255", true},
		{"::ffff:127.0.0.1", true}, {"::ffff:10.0.0.1", true}, {"::ffff:169.254.169.254", true}, {"::ffff:100.64.0.1", true},
		{"64:ff9b::a00:1", true}, {"2002:a00:1::", true}, {"fe80::1%eth0", true},
		{"8.8.8.8", false}, {"1.1.1.1", false}, {"172.32.0.1", false}, {"100.63.255.255", false}, {"100.128.0.1", false},
		{"2606:4700:4700::1111", false}, {"::ffff:8.8.8.8", false}, {"64:ff9b::808:808", false},
	}
	for _, tc := range tests {
		ip, err := netip.ParseAddr(tc.ip)
		if err != nil {
			t.Fatalf("%s: %v", tc.ip, err)
		}
		if got := classify(ip) != ""; got != tc.want {
			t.Errorf("classify(%s) refused = %v, want %v (%q)", tc.ip, got, tc.want, classify(ip))
		}
	}
}
