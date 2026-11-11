package objstore

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var ErrNotFound = errors.New("objstore: object not found")

type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

type Store struct {
	bucket   string
	client   *s3.Client
	transfer *transfermanager.Client
}

func New(cfg Config) *Store {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: true,
	})
	transfer := transfermanager.New(client, func(o *transfermanager.Options) { o.Concurrency = 2 })
	return &Store{bucket: cfg.Bucket, client: client, transfer: transfer}
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader) error {
	_, err := s.transfer.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("objstore: put %s: %w", key, err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("objstore: get %s: %w", key, err)
	}
	return out.Body, nil
}

func (s *Store) Check(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("objstore: bucket %s unavailable: %w", s.bucket, err)
	}
	return nil
}
