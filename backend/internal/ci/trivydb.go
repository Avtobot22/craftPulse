package ci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Files of the Trivy cache directory: Trivy keeps its vulnerability database in db/, craftpulse.json next to
// it records the content hash and the source of the snapshot.
const (
	_trivyDBDir      = "db"
	_trivyDBFile     = "trivy.db"
	_trivyDBMetadata = "metadata.json"
	_trivyDBSnapshot = "craftpulse.json"
	_trivyDBTimeout  = 15 * time.Minute
)

// ErrTrivyDBMissing is returned when the cache directory has no complete database snapshot.
var ErrTrivyDBMissing = errors.New("trivy vulnerability database is missing or incomplete")

// TrivyDB is the vulnerability database snapshot a scan uses: its version for the scan manifest and the
// time Trivy published it.
type TrivyDB struct {
	Version     string
	PublishedAt time.Time
	Repository  string
}

type trivyMetadata struct {
	Version   int       `json:"Version"`   //nolint:tagliatelle // Trivy wire format.
	UpdatedAt time.Time `json:"UpdatedAt"` //nolint:tagliatelle // Trivy wire format.
}

type trivySnapshot struct {
	SHA256     string    `json:"sha256"`
	Repository string    `json:"repository"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ReadTrivyDB reads the snapshot in the cache directory. The version is the schema version of the database
// and the SHA-256 of its content; the snapshot record must agree with the metadata of Trivy.
func ReadTrivyDB(cacheDir string) (TrivyDB, error) {
	dir := filepath.Join(cacheDir, _trivyDBDir)
	var meta trivyMetadata
	if err := readJSON(filepath.Join(dir, _trivyDBMetadata), &meta); err != nil {
		return TrivyDB{}, fmt.Errorf("%w: %v", ErrTrivyDBMissing, err) //nolint:errorlint // the cause is detail of the sentinel
	}
	var snap trivySnapshot
	if err := readJSON(filepath.Join(dir, _trivyDBSnapshot), &snap); err != nil {
		return TrivyDB{}, fmt.Errorf("%w: %v", ErrTrivyDBMissing, err) //nolint:errorlint // the cause is detail of the sentinel
	}
	if info, err := os.Stat(filepath.Join(dir, _trivyDBFile)); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return TrivyDB{}, fmt.Errorf("%w: no %s", ErrTrivyDBMissing, _trivyDBFile)
	}
	if meta.Version <= 0 || meta.UpdatedAt.IsZero() || len(snap.SHA256) != 64 || !snap.UpdatedAt.Equal(meta.UpdatedAt) {
		return TrivyDB{}, fmt.Errorf("%w: the snapshot record does not match the database metadata", ErrTrivyDBMissing)
	}
	return TrivyDB{
		Version: strconv.Itoa(meta.Version) + "@sha256:" + snap.SHA256, PublishedAt: meta.UpdatedAt.UTC(), Repository: snap.Repository,
	}, nil
}

// UpdateTrivyDB downloads the database from the OCI repository into a new directory next to the current one,
// checks and hashes it and only then replaces the current snapshot. A failed update keeps the current snapshot.
func (r *Runner) UpdateTrivyDB(ctx context.Context, cacheDir, repository string) (TrivyDB, error) {
	if repository == "" {
		return TrivyDB{}, errors.New("the repository of the vulnerability database is required")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil { //nolint:gosec // the database is not secret and other tools read it
		return TrivyDB{}, fmt.Errorf("create cache directory: %w", err)
	}
	work, err := os.MkdirTemp(cacheDir, ".update-")
	if err != nil {
		return TrivyDB{}, fmt.Errorf("create update directory: %w", err)
	}
	defer os.RemoveAll(work)
	t := Tool{
		Command: []string{
			ToolTrivy, "image", "--download-db-only", "--cache-dir", work, "--db-repository", repository,
			"--no-progress", "--skip-version-check",
		},
		Timeout: _trivyDBTimeout,
	}
	code, timedOut, err := r.exec(ctx, &t)
	switch {
	case ctx.Err() != nil:
		return TrivyDB{}, ctx.Err()
	case timedOut:
		return TrivyDB{}, errors.New("download of the vulnerability database ran out of time")
	case err != nil || code != 0:
		return TrivyDB{}, fmt.Errorf("download of the vulnerability database from %s failed", repository)
	}
	fresh := filepath.Join(work, _trivyDBDir)
	var meta trivyMetadata
	if err := readJSON(filepath.Join(fresh, _trivyDBMetadata), &meta); err != nil || meta.UpdatedAt.IsZero() {
		return TrivyDB{}, errors.New("the downloaded database has no metadata")
	}
	sum, err := fileSHA256(filepath.Join(fresh, _trivyDBFile))
	if err != nil {
		return TrivyDB{}, err
	}
	data, err := json.Marshal(trivySnapshot{SHA256: sum, Repository: repository, UpdatedAt: meta.UpdatedAt})
	if err != nil {
		return TrivyDB{}, fmt.Errorf("encode snapshot record: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fresh, _trivyDBSnapshot), data, 0o644); err != nil { //nolint:gosec // see MkdirAll
		return TrivyDB{}, fmt.Errorf("write snapshot record: %w", err)
	}
	current := filepath.Join(cacheDir, _trivyDBDir)
	if err := os.Rename(current, filepath.Join(work, "previous")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return TrivyDB{}, fmt.Errorf("move the current database: %w", err)
	}
	if err := os.Rename(fresh, current); err != nil {
		_ = os.Rename(filepath.Join(work, "previous"), current) // best effort: the error below is what the step reports
		return TrivyDB{}, fmt.Errorf("install the downloaded database: %w", err)
	}
	return ReadTrivyDB(cacheDir)
}

func fileSHA256(name string) (string, error) {
	f, err := os.Open(name) //nolint:gosec // the database file of the cache directory
	if err != nil {
		return "", fmt.Errorf("read vulnerability database: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash vulnerability database: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
