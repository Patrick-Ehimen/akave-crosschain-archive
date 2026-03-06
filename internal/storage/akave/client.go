package akave

import (
	"context"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
)

const (
	maxRetries    = 3
	baseBackoff   = 1 * time.Second
	backoffFactor = 2
)

// ObjectInfo holds metadata about a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// MinioClient defines the subset of minio.Client methods used by Client.
// This enables mocking in tests.
type MinioClient interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error)
	ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

// Client wraps a MinIO S3-compatible client for Akave O3 storage.
type Client struct {
	minio      MinioClient
	bucketName string
	log        zerolog.Logger
}

// NewClient creates a new Akave O3 client and ensures the configured bucket exists.
func NewClient(ctx context.Context, cfg config.Akave, log zerolog.Logger) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("creating minio client: %w", err)
	}

	c := &Client{
		minio:      mc,
		bucketName: cfg.BucketName,
		log:        log.With().Str("component", "akave").Logger(),
	}

	if err := c.ensureBucket(ctx); err != nil {
		return nil, err
	}

	c.log.Info().
		Str("endpoint", cfg.Endpoint).
		Str("bucket", cfg.BucketName).
		Msg("Connected to Akave O3 storage")

	return c, nil
}

// newClientFromMinio creates a Client from an existing MinioClient (used in tests).
func newClientFromMinio(mc MinioClient, bucketName string, log zerolog.Logger) *Client {
	return &Client{
		minio:      mc,
		bucketName: bucketName,
		log:        log.With().Str("component", "akave").Logger(),
	}
}

func (c *Client) ensureBucket(ctx context.Context) error {
	exists, err := c.minio.BucketExists(ctx, c.bucketName)
	if err != nil {
		return fmt.Errorf("checking bucket existence: %w", err)
	}
	if !exists {
		if err := c.minio.MakeBucket(ctx, c.bucketName, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("creating bucket %q: %w", c.bucketName, err)
		}
		c.log.Info().Str("bucket", c.bucketName).Msg("Created bucket")
	}
	return nil
}

// Upload stores data at the given key with retry logic.
func (c *Client) Upload(ctx context.Context, key string, reader io.Reader, size int64) error {
	return c.withRetry(ctx, func(ctx context.Context) error {
		_, err := c.minio.PutObject(ctx, c.bucketName, key, reader, size, minio.PutObjectOptions{})
		return err
	})
}

// Download retrieves the object at the given key with retry logic.
func (c *Client) Download(ctx context.Context, key string) (*minio.Object, error) {
	var obj *minio.Object
	err := c.withRetry(ctx, func(ctx context.Context) error {
		var err error
		obj, err = c.minio.GetObject(ctx, c.bucketName, key, minio.GetObjectOptions{})
		return err
	})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// List returns all objects matching the given prefix.
func (c *Client) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo

	ch := c.minio.ListObjects(ctx, c.bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range ch {
		if obj.Err != nil {
			return nil, fmt.Errorf("listing objects: %w", obj.Err)
		}
		objects = append(objects, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}

	return objects, nil
}

// Delete removes the object at the given key.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.minio.RemoveObject(ctx, c.bucketName, key, minio.RemoveObjectOptions{})
}

// withRetry executes fn with exponential backoff retries.
func (c *Client) withRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt < maxRetries-1 {
			backoff := time.Duration(float64(baseBackoff) * math.Pow(backoffFactor, float64(attempt)))
			c.log.Warn().
				Err(lastErr).
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Msg("O3 operation failed, retrying")

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}
