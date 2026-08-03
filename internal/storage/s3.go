package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config configures an S3ObjectStore against any S3-compatible service.
// Only the endpoint, credentials, region, and SSL flag differ between MinIO
// (development), Cloudflare R2, and AWS S3 (production) — the adapter is
// identical.
type S3Config struct {
	// Endpoint is the service URL, e.g. "minio:9000" in development or
	// "https://<account>.r2.cloudflarestorage.com" for Cloudflare R2.
	Endpoint string
	// AccessKey and SecretKey are the service credentials.
	AccessKey string
	SecretKey string
	// Region is the service region — "auto" for Cloudflare R2, "us-east-1"
	// for AWS S3, "us-east-1" for MinIO.
	Region string
	// Bucket is the container for all objects. Object keys are still
	// tenant-scoped within the bucket.
	Bucket string
	// UseSSL controls whether a scheme-less Endpoint is served over TLS.
	UseSSL bool
}

// S3ObjectStore implements ObjectStore backed by any S3-compatible service:
// MinIO in development, Cloudflare R2 or AWS S3 in production.
type S3ObjectStore struct {
	client *s3.Client
	bucket string
}

// NewS3ObjectStore builds an S3ObjectStore and ensures the bucket exists.
func NewS3ObjectStore(cfg *S3Config) (*S3ObjectStore, error) {
	client := s3.New(s3.Options{
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		BaseEndpoint: aws.String(normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)),
		UsePathStyle: true,
	})

	store := &S3ObjectStore{client: client, bucket: cfg.Bucket}
	if err := store.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

// normalizeEndpoint prefixes a scheme-less endpoint with http:// or https://
// based on UseSSL. Qualified endpoints (already containing a scheme) are used
// as-is.
func normalizeEndpoint(endpoint string, useSSL bool) string {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return scheme + "://" + endpoint
}

// ensureBucket creates the bucket if it does not already exist. Creating an
// existing bucket is not an error.
func (s *S3ObjectStore) ensureBucket(ctx context.Context) error {
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	var ownedByYou *types.BucketAlreadyOwnedByYou
	if errors.As(err, &ownedByYou) {
		return nil
	}
	var alreadyExists *types.BucketAlreadyExists
	if errors.As(err, &alreadyExists) {
		return nil
	}
	return fmt.Errorf("ensure bucket %q: %w", s.bucket, err)
}

// Put implements ObjectStore.
func (s *S3ObjectStore) Put(ctx context.Context, tenantID domain.TenantID, key string, r io.Reader, size int64) error {
	fullKey, err := objectKey(tenantID, key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(fullKey),
		Body:          r,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("put object %q: %w", fullKey, err)
	}
	return nil
}

// Get implements ObjectStore.
func (s *S3ObjectStore) Get(ctx context.Context, tenantID domain.TenantID, key string) (io.ReadCloser, error) {
	fullKey, err := objectKey(tenantID, key)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		var notFound *types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get object %q: %w", fullKey, err)
	}
	return out.Body, nil
}

// Delete implements ObjectStore.
func (s *S3ObjectStore) Delete(ctx context.Context, tenantID domain.TenantID, key string) error {
	fullKey, err := objectKey(tenantID, key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("delete object %q: %w", fullKey, err)
	}
	return nil
}
