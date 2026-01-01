package storage_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/oladayo21/letterbox/internal/storage"
)

const (
	testEndpoint  = "http://localhost:9000"
	testBucket    = "letterbox-test"
	testAccessKey = "minioadmin"
	testSecretKey = "minioadmin"
	testRegion    = "us-east-1"
)

func skipIfMinioUnavailable(t *testing.T) {
	t.Helper()

	// Check if Minio is running
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(testEndpoint + "/minio/health/live")

	if err != nil {
		t.Skipf("Minio not available at %s: %v", testEndpoint, err)
	}

	resp.Body.Close()
}

func setupTestBucket(t *testing.T) {
	t.Helper()

	client := s3.New(s3.Options{
		Region:       testRegion,
		BaseEndpoint: aws.String(testEndpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(testAccessKey, testSecretKey, ""),
		UsePathStyle: true,
	})

	ctx := context.Background()

	// Create bucket if not exists
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(testBucket),
	})

	if err != nil {
		// Ignore "bucket already exists" error
		t.Logf("CreateBucket: %v (may already exist)", err)
	}
}

func newTestStorage(t *testing.T) *storage.S3Storage {
	t.Helper()

	skipIfMinioUnavailable(t)
	setupTestBucket(t)

	s, err := storage.NewS3Storage(storage.S3Config{
		Endpoint:  testEndpoint,
		Bucket:    testBucket,
		AccessKey: testAccessKey,
		SecretKey: testSecretKey,
		Region:    testRegion,
	})

	if err != nil {
		t.Fatalf("creating S3 storage: %v", err)
	}

	return s
}

func TestNewS3Storage_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		cfg    storage.S3Config
		errMsg string
	}{
		{
			name:   "missing endpoint",
			cfg:    storage.S3Config{Bucket: "b", AccessKey: "a", SecretKey: "s"},
			errMsg: "endpoint",
		},
		{
			name:   "missing bucket",
			cfg:    storage.S3Config{Endpoint: "e", AccessKey: "a", SecretKey: "s"},
			errMsg: "bucket",
		},
		{
			name:   "missing access key",
			cfg:    storage.S3Config{Endpoint: "e", Bucket: "b", SecretKey: "s"},
			errMsg: "credentials",
		},
		{
			name:   "missing secret key",
			cfg:    storage.S3Config{Endpoint: "e", Bucket: "b", AccessKey: "a"},
			errMsg: "credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := storage.NewS3Storage(tt.cfg)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestAttachmentKey(t *testing.T) {
	key := storage.AttachmentKey("acc-123", 456, "document.pdf")

	expected := "acc-123/456/document.pdf"

	if key != expected {
		t.Errorf("AttachmentKey = %q, want %q", key, expected)
	}
}

func TestS3Storage_Upload(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	key := "test-upload/" + time.Now().Format("20060102150405") + "/file.txt"
	data := []byte("Hello, World!")
	contentType := "text/plain"

	url, err := s.Upload(ctx, key, data, contentType)

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if url == "" {
		t.Error("Upload returned empty URL")
	}

	// Verify file exists
	exists, err := s.Exists(ctx, key)

	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}

	if !exists {
		t.Error("Uploaded file does not exist")
	}

	// Cleanup
	_ = s.Delete(ctx, key)
}

func TestS3Storage_GeneratePresignedURL(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// First upload a file
	key := "test-presign/" + time.Now().Format("20060102150405") + "/file.txt"
	data := []byte("Presigned content")

	_, err := s.Upload(ctx, key, data, "text/plain")

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Generate presigned URL
	url, err := s.GeneratePresignedURL(ctx, key, 15*time.Minute)

	if err != nil {
		t.Fatalf("GeneratePresignedURL failed: %v", err)
	}

	if url == "" {
		t.Error("GeneratePresignedURL returned empty URL")
	}

	// Verify URL is accessible
	resp, err := http.Get(url)

	if err != nil {
		t.Fatalf("GET presigned URL failed: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Presigned URL returned status %d, want 200", resp.StatusCode)
	}

	// Cleanup
	_ = s.Delete(ctx, key)
}

func TestS3Storage_Delete(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	key := "test-delete/" + time.Now().Format("20060102150405") + "/file.txt"

	// Upload
	_, err := s.Upload(ctx, key, []byte("to delete"), "text/plain")

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify exists
	exists, _ := s.Exists(ctx, key)

	if !exists {
		t.Fatal("File should exist after upload")
	}

	// Delete
	err = s.Delete(ctx, key)

	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	exists, _ = s.Exists(ctx, key)

	if exists {
		t.Error("File should not exist after delete")
	}
}

func TestS3Storage_Exists_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	exists, err := s.Exists(ctx, "nonexistent/file.txt")

	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if exists {
		t.Error("Nonexistent file should return false")
	}
}

func TestS3Storage_UploadLargeFile(t *testing.T) {
	if os.Getenv("RUN_LARGE_FILE_TESTS") == "" {
		t.Skip("Skipping large file test (set RUN_LARGE_FILE_TESTS=1)")
	}

	s := newTestStorage(t)
	ctx := context.Background()

	// 5MB file
	key := "test-large/" + time.Now().Format("20060102150405") + "/large.bin"
	data := make([]byte, 5*1024*1024)

	for i := range data {
		data[i] = byte(i % 256)
	}

	url, err := s.Upload(ctx, key, data, "application/octet-stream")

	if err != nil {
		t.Fatalf("Upload large file failed: %v", err)
	}

	if url == "" {
		t.Error("Upload returned empty URL")
	}

	// Cleanup
	_ = s.Delete(ctx, key)
}

func TestS3Storage_Bucket(t *testing.T) {
	skipIfMinioUnavailable(t)

	s, _ := storage.NewS3Storage(storage.S3Config{
		Endpoint:  testEndpoint,
		Bucket:    "my-bucket",
		AccessKey: testAccessKey,
		SecretKey: testSecretKey,
	})

	if s.Bucket() != "my-bucket" {
		t.Errorf("Bucket() = %q, want %q", s.Bucket(), "my-bucket")
	}
}
