package blobstore

import (
	"context"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCSStore stores blobs in Google Cloud Storage.
type GCSStore struct {
	client *storage.Client
	bucket string
}

// NewGCSStore creates a new GCS-backed blob store.
// If credJSON is provided, uses explicit credentials. Otherwise uses default credentials.
func NewGCSStore(ctx context.Context, bucket string, credJSON ...[]byte) (*GCSStore, error) {
	var client *storage.Client
	var err error

	if len(credJSON) > 0 && len(credJSON[0]) > 0 {
		client, err = storage.NewClient(ctx, option.WithCredentialsJSON(credJSON[0]))
	} else {
		client, err = storage.NewClient(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("creating GCS client: %w", err)
	}
	return &GCSStore{client: client, bucket: bucket}, nil
}

func (s *GCSStore) objectPath(repoID, key string) string {
	return fmt.Sprintf("%s/%s", repoID, key)
}

// Put stores a blob in GCS.
func (s *GCSStore) Put(ctx context.Context, repoID, key string, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	path := s.objectPath(repoID, key)
	w := s.client.Bucket(s.bucket).Object(path).NewWriter(ctx)
	w.ContentType = "application/octet-stream"

	if _, err := w.Write(data); err != nil {
		w.Close()
		return fmt.Errorf("writing to GCS %s: %w", path, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing GCS writer %s: %w", path, err)
	}
	return nil
}

// Get retrieves a blob from GCS.
func (s *GCSStore) Get(ctx context.Context, repoID, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	path := s.objectPath(repoID, key)
	r, err := s.client.Bucket(s.bucket).Object(path).NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, fmt.Errorf("blob not found: %s", path)
		}
		return nil, fmt.Errorf("reading from GCS %s: %w", path, err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading GCS body %s: %w", path, err)
	}
	return data, nil
}

// GetRange retrieves a byte range from a blob in GCS.
// Uses HTTP Range requests — only downloads the needed bytes.
func (s *GCSStore) GetRange(ctx context.Context, repoID, key string, offset, length int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	path := s.objectPath(repoID, key)
	r, err := s.client.Bucket(s.bucket).Object(path).NewRangeReader(ctx, offset, length)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, fmt.Errorf("blob not found: %s", path)
		}
		return nil, fmt.Errorf("range read from GCS %s: %w", path, err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading GCS range %s: %w", path, err)
	}
	return data, nil
}

// Delete removes a blob from GCS.
func (s *GCSStore) Delete(ctx context.Context, repoID, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	path := s.objectPath(repoID, key)
	err := s.client.Bucket(s.bucket).Object(path).Delete(ctx)
	if err != nil && err != storage.ErrObjectNotExist {
		return fmt.Errorf("deleting from GCS %s: %w", path, err)
	}
	return nil
}

// Close closes the GCS client.
func (s *GCSStore) Close() error {
	return s.client.Close()
}
