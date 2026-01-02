package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Storage handles attachment storage in S3-compatible storage.
type S3Storage struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	endpoint      string
}

// S3Config contains configuration for S3 storage.
type S3Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
}

// NewS3Storage creates a new S3 storage client.
func NewS3Storage(cfg S3Config) (*S3Storage, error) {

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("S3 endpoint is required")
	}

	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}

	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("S3 credentials are required")
	}

	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	client := s3.New(s3.Options{
		Region:       cfg.Region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: true, // Required for Minio
	})

	return &S3Storage{
		client:        client,
		presignClient: s3.NewPresignClient(client),
		bucket:        cfg.Bucket,
		endpoint:      cfg.Endpoint,
	}, nil
}

// AttachmentKey generates the S3 key for an attachment.
// Format: {account_id}/{email_uid}/{filename}
// Filename is sanitized to prevent path traversal attacks.
func AttachmentKey(accountID string, emailUID int64, filename string) string {
	// Sanitize filename to prevent path traversal (e.g., "../../../secret.txt")
	safeFilename := filepath.Base(filename)

	return fmt.Sprintf("%s/%d/%s", accountID, emailUID, safeFilename)
}

// Upload uploads data to S3 and returns the object URL.
func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := s.client.PutObject(ctx, input)

	if err != nil {
		return "", fmt.Errorf("uploading to S3: %w", err)
	}

	// Return the object URL (not presigned, just the path)
	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)

	return url, nil
}

// GeneratePresignedURL generates a presigned URL for downloading an object.
func (s *S3Storage) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	presigned, err := s.presignClient.PresignGetObject(ctx, input, s3.WithPresignExpires(expiry))

	if err != nil {
		return "", fmt.Errorf("generating presigned URL: %w", err)
	}

	return presigned.URL, nil
}

// Delete removes an object from S3.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err := s.client.DeleteObject(ctx, input)

	if err != nil {
		return fmt.Errorf("deleting from S3: %w", err)
	}

	return nil
}

// Exists checks if an object exists in S3.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err := s.client.HeadObject(ctx, input)

	if err != nil {
		// Check specifically for NotFound error
		var notFound *types.NotFound

		if errors.As(err, &notFound) {
			return false, nil
		}

		// Also check for NoSuchKey (some S3-compatible services use this)
		var noSuchKey *types.NoSuchKey

		if errors.As(err, &noSuchKey) {
			return false, nil
		}

		// Propagate other errors (network, auth, etc.)
		return false, fmt.Errorf("checking S3 object existence: %w", err)
	}

	return true, nil
}

// Bucket returns the configured bucket name.
func (s *S3Storage) Bucket() string {

	return s.bucket
}

// Ping checks if the S3 storage is accessible by verifying bucket access.
func (s *S3Storage) Ping(ctx context.Context) error {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	}

	_, err := s.client.HeadBucket(ctx, input)

	if err != nil {
		return fmt.Errorf("S3 bucket not accessible: %w", err)
	}

	return nil
}
