package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/goosemooz/something-backend/config"
)

type s3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type Storage struct {
	client    s3Client
	pdfBucket string
	pfpBucket string
}

func New(cfg *appconfig.Config) (*Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKey,
			cfg.S3SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load S3 config: %w", err)
	}

	return &Storage{
		client:    s3.NewFromConfig(awsCfg),
		pdfBucket: cfg.S3PDFBucket,
		pfpBucket: cfg.S3PFPBucket,
	}, nil
}

func (s *Storage) UploadPDF(ctx context.Context, key string, body []byte) (string, error) {
	return s.upload(ctx, s.pdfBucket, key, body, "application/pdf")
}

func (s *Storage) UploadPFP(ctx context.Context, key string, body []byte, contentType string) (string, error) {
	return s.upload(ctx, s.pfpBucket, key, body, contentType)
}

func (s *Storage) upload(ctx context.Context, bucket, key string, body []byte, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, key), nil
}

func (s *Storage) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}
	return nil
}

func (s *Storage) DeleteOwnedURL(ctx context.Context, rawURL string) error {
	bucket, key, ok := s.ParseOwnedURL(rawURL)
	if !ok {
		return nil
	}
	return s.Delete(ctx, bucket, key)
}

func (s *Storage) ParseOwnedURL(rawURL string) (bucket, key string, ok bool) {
	if rawURL == "" {
		return "", "", false
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return "", "", false
	}

	host := u.Hostname()
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	switch host {
	case s.pdfBucket + ".s3.amazonaws.com":
		if path == "" {
			return "", "", false
		}
		return s.pdfBucket, path, true
	case s.pfpBucket + ".s3.amazonaws.com":
		if path == "" {
			return "", "", false
		}
		return s.pfpBucket, path, true
	default:
		return "", "", false
	}
}
