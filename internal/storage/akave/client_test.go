package akave

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/rs/zerolog"
)

// mockMinioClient implements MinioClient for testing.
type mockMinioClient struct {
	bucketExists    bool
	bucketExistsErr error
	makeBucketErr   error
	putObjectErr    error
	putObjectCalls  int
	getObjectObj    *minio.Object
	getObjectErr    error
	getObjectCalls  int
	listObjects     []minio.ObjectInfo
	removeObjectErr error
}

func (m *mockMinioClient) BucketExists(_ context.Context, _ string) (bool, error) {
	return m.bucketExists, m.bucketExistsErr
}

func (m *mockMinioClient) MakeBucket(_ context.Context, _ string, _ minio.MakeBucketOptions) error {
	return m.makeBucketErr
}

func (m *mockMinioClient) PutObject(_ context.Context, _, _ string, _ io.Reader, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	m.putObjectCalls++
	return minio.UploadInfo{}, m.putObjectErr
}

func (m *mockMinioClient) GetObject(_ context.Context, _, _ string, _ minio.GetObjectOptions) (*minio.Object, error) {
	m.getObjectCalls++
	return m.getObjectObj, m.getObjectErr
}

func (m *mockMinioClient) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo, len(m.listObjects))
	for _, obj := range m.listObjects {
		ch <- obj
	}
	close(ch)
	return ch
}

func (m *mockMinioClient) RemoveObject(_ context.Context, _, _ string, _ minio.RemoveObjectOptions) error {
	return m.removeObjectErr
}

func newTestClient(mock *mockMinioClient) *Client {
	log := zerolog.Nop()
	return newClientFromMinio(mock, "test-bucket", log)
}

func TestEnsureBucket_AlreadyExists(t *testing.T) {
	mock := &mockMinioClient{bucketExists: true}
	c := newTestClient(mock)

	err := c.ensureBucket(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureBucket_Created(t *testing.T) {
	mock := &mockMinioClient{bucketExists: false}
	c := newTestClient(mock)

	err := c.ensureBucket(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureBucket_ExistsError(t *testing.T) {
	mock := &mockMinioClient{bucketExistsErr: fmt.Errorf("network error")}
	c := newTestClient(mock)

	err := c.ensureBucket(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEnsureBucket_MakeBucketError(t *testing.T) {
	mock := &mockMinioClient{bucketExists: false, makeBucketErr: fmt.Errorf("permission denied")}
	c := newTestClient(mock)

	err := c.ensureBucket(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpload_Success(t *testing.T) {
	mock := &mockMinioClient{}
	c := newTestClient(mock)

	data := bytes.NewReader([]byte("hello"))
	err := c.Upload(context.Background(), "test/key", data, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.putObjectCalls != 1 {
		t.Errorf("expected 1 PutObject call, got %d", mock.putObjectCalls)
	}
}

func TestUpload_RetryOnError(t *testing.T) {
	// Mock that always fails — should retry 3 times.
	mock := &mockMinioClient{putObjectErr: fmt.Errorf("transient error")}
	c := newTestClient(mock)

	data := bytes.NewReader([]byte("hello"))
	err := c.Upload(context.Background(), "test/key", data, 5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if mock.putObjectCalls != maxRetries {
		t.Errorf("expected %d PutObject calls, got %d", maxRetries, mock.putObjectCalls)
	}
}

func TestUpload_ContextCancelled(t *testing.T) {
	mock := &mockMinioClient{putObjectErr: fmt.Errorf("transient error")}
	c := newTestClient(mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	data := bytes.NewReader([]byte("hello"))
	err := c.Upload(ctx, "test/key", data, 5)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDownload_Success(t *testing.T) {
	mock := &mockMinioClient{}
	c := newTestClient(mock)

	_, err := c.Download(context.Background(), "test/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.getObjectCalls != 1 {
		t.Errorf("expected 1 GetObject call, got %d", mock.getObjectCalls)
	}
}

func TestDownload_RetryOnError(t *testing.T) {
	mock := &mockMinioClient{getObjectErr: fmt.Errorf("transient error")}
	c := newTestClient(mock)

	_, err := c.Download(context.Background(), "test/key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if mock.getObjectCalls != maxRetries {
		t.Errorf("expected %d GetObject calls, got %d", maxRetries, mock.getObjectCalls)
	}
}

func TestList_Success(t *testing.T) {
	mock := &mockMinioClient{
		listObjects: []minio.ObjectInfo{
			{Key: "prefix/a.parquet", Size: 100, LastModified: time.Now()},
			{Key: "prefix/b.parquet", Size: 200, LastModified: time.Now()},
		},
	}
	c := newTestClient(mock)

	objects, err := c.List(context.Background(), "prefix/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 2 {
		t.Errorf("got %d objects, want 2", len(objects))
	}
	if objects[0].Key != "prefix/a.parquet" {
		t.Errorf("got key %q, want %q", objects[0].Key, "prefix/a.parquet")
	}
}

func TestList_Empty(t *testing.T) {
	mock := &mockMinioClient{listObjects: []minio.ObjectInfo{}}
	c := newTestClient(mock)

	objects, err := c.List(context.Background(), "nonexistent/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objects) != 0 {
		t.Errorf("got %d objects, want 0", len(objects))
	}
}

func TestList_Error(t *testing.T) {
	mock := &mockMinioClient{
		listObjects: []minio.ObjectInfo{
			{Err: fmt.Errorf("listing failed")},
		},
	}
	c := newTestClient(mock)

	_, err := c.List(context.Background(), "prefix/")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDelete_Success(t *testing.T) {
	mock := &mockMinioClient{}
	c := newTestClient(mock)

	err := c.Delete(context.Background(), "test/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_Error(t *testing.T) {
	mock := &mockMinioClient{removeObjectErr: fmt.Errorf("not found")}
	c := newTestClient(mock)

	err := c.Delete(context.Background(), "test/key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
