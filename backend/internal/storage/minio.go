package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"backend/pkg/logger"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

type MinioStorage struct {
	Client        *minio.Client
	Bucket        string
	tempPrefix    string
	objectsPrefix string
}

func NewMinioStorage() (*MinioStorage, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	access := os.Getenv("MINIO_ACCESS_KEY")
	secret := os.Getenv("MINIO_SECRET_KEY")
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "filevault"
	}
	useSSL := false
	if os.Getenv("MINIO_USE_SSL") == "true" {
		useSSL = true
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	if err != nil {
		exists, e := client.BucketExists(ctx, bucket)
		if e != nil || !exists {
			return nil, fmt.Errorf("create bucket error: %v (exists? %v)", err, exists)
		}
	}
	s := &MinioStorage{
		Client:        client,
		Bucket:        bucket,
		tempPrefix:    "tmp/",
		objectsPrefix: "objects/",
	}
	return s, nil
}

func (s *MinioStorage) PresignedPutURL(ctx context.Context, objectName string, ttlMinutes int64) (string, error) {
	presignedURL, err := s.Client.PresignedPutObject(ctx, s.Bucket, objectName, time.Duration(ttlMinutes)*time.Minute)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (s *MinioStorage) PresignedGetURL(ctx context.Context, objectName string, ttlMinutes int64) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := s.Client.PresignedGetObject(ctx, s.Bucket, objectName, time.Duration(ttlMinutes)*time.Minute, reqParams)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (s *MinioStorage) CopyTempToObject(ctx context.Context, tempName, finalName string) error {
	src := minio.CopySrcOptions{
		Bucket: s.Bucket,
		Object: tempName,
	}
	dst := minio.CopyDestOptions{
		Bucket: s.Bucket,
		Object: finalName,
	}
	_, err := s.Client.CopyObject(ctx, dst, src)
	if err != nil {
		return err
	}
	err = s.Client.RemoveObject(ctx, s.Bucket, tempName, minio.RemoveObjectOptions{})
	if err != nil {
		logger.L.Warn("failed to remove temp object", zap.String("object", tempName), zap.Error(err))
	}
	return nil
}

func (s *MinioStorage) GetObjectReader(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj, err := s.Client.GetObject(ctx, s.Bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	_, err = obj.Stat()
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func MinioRemoveOpts() minio.RemoveObjectOptions {
	return minio.RemoveObjectOptions{GovernanceBypass: true}
}
