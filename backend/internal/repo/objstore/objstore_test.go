package objstore_test

import (
	"crypto/sha256"
	"io"
	"math/rand/v2"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/objstore"
)

const _objectSize = 100 << 20

func startSeaweed(t *testing.T) objstore.Config {
	t.Helper()
	c, err := testcontainers.Run(t.Context(), "chrislusf/seaweedfs:4.53",
		testcontainers.WithCmd("server", "-s3", "-dir=/data"),
		testcontainers.WithExposedPorts("8333/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("8333/tcp").WithStartupTimeout(time.Minute)),
	)
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)
	endpoint, err := c.PortEndpoint(t.Context(), "8333/tcp", "http")
	require.NoError(t, err)

	cfg := objstore.Config{Endpoint: endpoint, Region: "us-east-1", Bucket: "packages", AccessKeyID: "test", SecretAccessKey: "test"}
	admin := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint), Region: cfg.Region, UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	require.Eventually(t, func() bool {
		_, err := admin.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String(cfg.Bucket)})
		return err == nil
	}, 30*time.Second, 500*time.Millisecond)
	return cfg
}

func peakHeap(stop <-chan struct{}) *atomic.Uint64 {
	var peak atomic.Uint64
	go func() {
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak.Load() {
					peak.Store(m.HeapAlloc)
				}
			}
		}
	}()
	return &peak
}

func TestStore_StreamsLargeObject(t *testing.T) {
	store := objstore.New(startSeaweed(t))
	ctx := t.Context()
	require.NoError(t, store.Check(ctx))

	// Frequent GC keeps garbage out of the peak, so it reflects data actually held.
	defer debug.SetGCPercent(debug.SetGCPercent(10))
	stop := make(chan struct{})
	peak := peakHeap(stop)
	want := sha256.New()
	src := io.TeeReader(io.LimitReader(rand.NewChaCha8([32]byte{1}), _objectSize), want)
	require.NoError(t, store.Put(ctx, "quarantine/big.tar.gz", src))

	body, err := store.Get(ctx, "quarantine/big.tar.gz")
	require.NoError(t, err)
	got := sha256.New()
	n, err := io.Copy(got, body)
	require.NoError(t, err)
	require.NoError(t, body.Close())
	close(stop)

	assert.Equal(t, int64(_objectSize), n)
	assert.Equal(t, want.Sum(nil), got.Sum(nil))
	assert.Less(t, peak.Load(), uint64(_objectSize*3/4), "the object is never held in memory whole")
}

func TestStore_MissingObject(t *testing.T) {
	store := objstore.New(startSeaweed(t))
	_, err := store.Get(t.Context(), "nope")
	assert.ErrorIs(t, err, objstore.ErrNotFound)
}
