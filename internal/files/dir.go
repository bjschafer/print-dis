package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Dir keeps objects as files below a root directory.
type Dir struct {
	root *os.Root
}

// NewDir opens (creating if needed) dir as the store's root.
func NewDir(dir string) (*Dir, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("files dir: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("files dir: %w", err)
	}
	return &Dir{root: root}, nil
}

// Close releases the root directory handle.
func (d *Dir) Close() error { return d.root.Close() }

// checkKey accepts only clean, relative, slash-separated paths.
func checkKey(key string) error {
	if key == "." || !fs.ValidPath(key) || strings.ContainsAny(key, "\\\x00") {
		return fmt.Errorf("%w: %q", ErrBadKey, key)
	}
	return nil
}

func (d *Dir) Put(ctx context.Context, key string, r io.Reader, size int64, _ string) (err error) {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := path.Dir(key)
	if dir != "." {
		if err := d.root.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	tmp := path.Join(dir, ".tmp-"+hex.EncodeToString(rnd[:]))
	f, err := d.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = d.root.Remove(tmp)
		}
	}()
	n, err := io.Copy(f, r)
	if err != nil {
		return err
	}
	if size >= 0 && n != size {
		err = fmt.Errorf("files: wrote %d bytes, expected %d", n, size)
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	err = d.root.Rename(tmp, key)
	return err
}

func (d *Dir) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	if err := checkKey(key); err != nil {
		return nil, 0, err
	}
	f, err := d.root.Open(key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		_ = f.Close()
		if err == nil {
			err = ErrNotFound
		}
		return nil, 0, err
	}
	return f, st.Size(), nil
}

func (d *Dir) Delete(_ context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	err := d.root.Remove(key)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
