package storage

import (
	"bytes"
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3Client struct {
	putInput    *s3.PutObjectInput
	deleteInput *s3.DeleteObjectInput
}

func (f *fakeS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putInput = params
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) DeleteObject(_ context.Context, params *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleteInput = params
	return &s3.DeleteObjectOutput{}, nil
}

func TestUploadPFPSetsContentHeadersAndBody(t *testing.T) {
	client := &fakeS3Client{}
	store := &Storage{
		client:    client,
		pfpBucket: "pfp-bucket",
	}

	url, err := store.UploadPFP(context.Background(), "pfp/users/test.gif", []byte("gif-data"), "image/gif")
	if err != nil {
		t.Fatalf("UploadPFP returned error: %v", err)
	}
	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	if client.putInput == nil {
		t.Fatal("expected PutObject to be called")
	}
	if got := *client.putInput.ContentType; got != "image/gif" {
		t.Fatalf("expected content type image/gif, got %q", got)
	}
	if got := *client.putInput.ContentLength; got != int64(len("gif-data")) {
		t.Fatalf("expected content length %d, got %d", len("gif-data"), got)
	}
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(client.putInput.Body); err != nil {
		t.Fatalf("reading uploaded body: %v", err)
	}
	if got := buf.String(); got != "gif-data" {
		t.Fatalf("expected uploaded body %q, got %q", "gif-data", got)
	}
}

func TestParseOwnedURL(t *testing.T) {
	store := &Storage{
		pdfBucket: "pdf-bucket",
		pfpBucket: "pfp-bucket",
	}

	bucket, key, ok := store.ParseOwnedURL("https://pfp-bucket.s3.amazonaws.com/pfp/users/test.gif")
	if !ok {
		t.Fatal("expected owned URL to parse")
	}
	if bucket != "pfp-bucket" || key != "pfp/users/test.gif" {
		t.Fatalf("unexpected parse result: %q %q", bucket, key)
	}

	if _, _, ok := store.ParseOwnedURL("https://example.com/file.gif"); ok {
		t.Fatal("expected foreign URL to be rejected")
	}
}

func TestDeleteOwnedURLDeletesParsedObject(t *testing.T) {
	client := &fakeS3Client{}
	store := &Storage{
		client:    client,
		pfpBucket: "pfp-bucket",
	}

	if err := store.DeleteOwnedURL(context.Background(), "https://pfp-bucket.s3.amazonaws.com/pfp/users/test.gif"); err != nil {
		t.Fatalf("DeleteOwnedURL returned error: %v", err)
	}
	if client.deleteInput == nil {
		t.Fatal("expected DeleteObject to be called")
	}
	if got := *client.deleteInput.Bucket; got != "pfp-bucket" {
		t.Fatalf("expected bucket pfp-bucket, got %q", got)
	}
	if got := *client.deleteInput.Key; got != "pfp/users/test.gif" {
		t.Fatalf("expected key pfp/users/test.gif, got %q", got)
	}
}
