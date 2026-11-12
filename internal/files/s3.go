package files

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Options configures the S3 backend.
type S3Options struct {
	Endpoint        string // a URL such as https://s3.example.com
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Region          string // may be empty
}

// S3 stores objects in an S3-compatible bucket, addressed path-style.
type S3 struct {
	c      *minio.Client
	bucket string
}

// NewS3 builds the client. It does not contact the server, so the app starts
// even when the bucket is unreachable.
func NewS3(o S3Options) (*S3, error) {
	u, err := url.Parse(o.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("S3_ENDPOINT %q must be an http(s) URL", o.Endpoint)
	}
	c, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(o.AccessKeyID, o.SecretAccessKey, ""),
		Secure:       u.Scheme == "https",
		Region:       o.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return &S3{c: c, bucket: o.Bucket}, nil
}

// partSize is the multipart chunk an upload is buffered in. It must be set:
// for a stream of unknown size minio-go otherwise assumes a 5 TiB object and
// buffers ~537 MiB per upload. 8 MiB stays above S3's 5 MiB minimum and
// gives 25 parts for a 200 MiB file, far below the 10,000-part limit.
const partSize = 8 << 20

// Get returns a *minio.Object, which reads by range.
var _ io.ReaderAt = (*minio.Object)(nil)

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	_, err := s.c.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType, PartSize: partSize})
	return err
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if err := checkKey(key); err != nil {
		return nil, 0, err
	}
	obj, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	st, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	return obj, st.Size, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	err := s.c.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && minio.ToErrorResponse(err).Code != minio.NoSuchKey {
		return err
	}
	return nil
}

// EnsureBucket creates the bucket if it does not exist. Used by tests.
func (s *S3) EnsureBucket(ctx context.Context) error {
	ok, err := s.c.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	err = s.c.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
	if err != nil && minio.ToErrorResponse(err).Code != "BucketAlreadyOwnedByYou" {
		return err
	}
	return nil
}
