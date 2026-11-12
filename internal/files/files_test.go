package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conformance runs against every backend.
func conformance(t *testing.T, b Blobs) {
	ctx := context.Background()
	read := func(key string) ([]byte, int64, error) {
		rc, n, err := b.Get(ctx, key)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = rc.Close() }()
		data, err := io.ReadAll(rc)
		return data, n, err
	}

	t.Run("random access", func(t *testing.T) {
		key := "requests/1/ra"
		if err := b.Put(ctx, key, strings.NewReader("0123456789"), -1, "model/3mf"); err != nil {
			t.Fatal(err)
		}
		rc, _, err := b.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rc.Close() }()
		ra, ok := rc.(io.ReaderAt)
		if !ok {
			t.Fatalf("%T is not an io.ReaderAt", rc)
		}
		p := make([]byte, 3)
		if _, err := ra.ReadAt(p, 6); err != nil || string(p) != "678" {
			t.Fatalf("ReadAt: %q %v", p, err)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		big := make([]byte, 3<<20)
		_, _ = rand.Read(big)
		for name, size := range map[string]int64{"known": int64(len(big)), "unknown": -1} {
			key := "requests/1/rt-" + name
			if err := b.Put(ctx, key, bytes.NewReader(big), size, "model/stl"); err != nil {
				t.Fatalf("put: %v", err)
			}
			got, n, err := read(key)
			if err != nil || n != int64(len(big)) || !bytes.Equal(got, big) {
				t.Fatalf("get: n=%d err=%v equal=%v", n, err, bytes.Equal(got, big))
			}
		}
	})
	t.Run("empty object", func(t *testing.T) {
		if err := b.Put(ctx, "requests/1/empty", bytes.NewReader(nil), 0, "text/plain"); err != nil {
			t.Fatal(err)
		}
		got, n, err := read("requests/1/empty")
		if err != nil || n != 0 || len(got) != 0 {
			t.Fatalf("n=%d err=%v", n, err)
		}
	})
	t.Run("overwrite", func(t *testing.T) {
		key := "requests/1/over"
		_ = b.Put(ctx, key, bytes.NewReader([]byte("first, longer")), -1, "text/plain")
		if err := b.Put(ctx, key, bytes.NewReader([]byte("second")), -1, "text/plain"); err != nil {
			t.Fatal(err)
		}
		got, n, err := read(key)
		if err != nil || string(got) != "second" || n != 6 {
			t.Fatalf("got %q n=%d err=%v", got, n, err)
		}
	})
	t.Run("failed put leaves nothing", func(t *testing.T) {
		key := "requests/1/failing"
		err := b.Put(ctx, key, io.MultiReader(bytes.NewReader([]byte("partial")), errReader{}), -1, "text/plain")
		if err == nil {
			t.Fatal("expected an error")
		}
		if _, _, err := b.Get(ctx, key); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get after failed put: %v", err)
		}
	})
	t.Run("delete", func(t *testing.T) {
		key := "requests/2/del"
		_ = b.Put(ctx, key, bytes.NewReader([]byte("x")), 1, "text/plain")
		if err := b.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
		if _, _, err := b.Get(ctx, key); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get after delete: %v", err)
		}
		if err := b.Delete(ctx, key); err != nil {
			t.Fatalf("deleting a missing object: %v", err)
		}
	})
	t.Run("get missing", func(t *testing.T) {
		if _, _, err := b.Get(ctx, "requests/9/nope"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})
	t.Run("bad keys", func(t *testing.T) {
		for _, key := range []string{"../x", "/abs", "a/../../b", "a/../b", "", ".", "a//b", "a\\b"} {
			if err := b.Put(ctx, key, bytes.NewReader([]byte("x")), 1, "text/plain"); !errors.Is(err, ErrBadKey) {
				t.Errorf("put %q: %v, want ErrBadKey", key, err)
			}
			if _, _, err := b.Get(ctx, key); !errors.Is(err, ErrBadKey) {
				t.Errorf("get %q: %v, want ErrBadKey", key, err)
			}
			if err := b.Delete(ctx, key); !errors.Is(err, ErrBadKey) {
				t.Errorf("delete %q: %v, want ErrBadKey", key, err)
			}
		}
	})
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestDir(t *testing.T) {
	d, err := NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	conformance(t, d)
}

func TestDirStaysInsideRoot(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "root"), 0o750); err != nil {
		t.Fatal(err)
	}
	d, err := NewDir(filepath.Join(parent, "root"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	_ = d.Put(context.Background(), "../escape", bytes.NewReader([]byte("x")), 1, "")
	if _, err := os.Stat(filepath.Join(parent, "escape")); err == nil {
		t.Fatal("wrote outside the root")
	}
}

func TestS3(t *testing.T) {
	endpoint, bucket := os.Getenv("TEST_S3_ENDPOINT"), os.Getenv("TEST_S3_BUCKET")
	ak, sk := os.Getenv("TEST_S3_ACCESS_KEY_ID"), os.Getenv("TEST_S3_SECRET_ACCESS_KEY")
	if endpoint == "" || bucket == "" || ak == "" || sk == "" {
		t.Skip("TEST_S3_ENDPOINT, TEST_S3_BUCKET, TEST_S3_ACCESS_KEY_ID and TEST_S3_SECRET_ACCESS_KEY not all set")
	}
	s, err := NewS3(S3Options{Endpoint: endpoint, Bucket: bucket, AccessKeyID: ak, SecretAccessKey: sk, Region: os.Getenv("TEST_S3_REGION")})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	conformance(t, s)
}
