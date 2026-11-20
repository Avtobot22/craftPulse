package ci

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_profileFile = "profile.json"
	_runsDir     = "runs"
	_reportsDir  = envelope.FilesDir + "reports"
)

var (
	_runID       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	_unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

	// ErrRunExists is returned when the job already recorded a tool run with this ID.
	ErrRunExists = errors.New("tool run with this ID is already recorded")
)

// Entry is one row of the check matrix of the project.
type Entry struct {
	ComponentID uuid.UUID `json:"component_id"`
	PathPrefix  string    `json:"path_prefix"`
	Language    string    `json:"language"`
	Kind        string    `json:"analysis_kind"`
	Mode        string    `json:"mode"`
	ProfileID   string    `json:"profile_id"`
	ImageDigest *string   `json:"image_digest"`
}

// Filter selects the rows of the matrix for the current job; an empty field matches any value.
type Filter struct {
	Kind, Mode, ProfileID, ComponentID string
}

// Select returns the rows of the profile that match the filter, in profile order.
func Select(p *ciapi.CiCheckProfile, f Filter) []Entry {
	var out []Entry
	for i := range p.Entries {
		e := &p.Entries[i]
		row := Entry{
			ComponentID: e.ComponentId, PathPrefix: e.PathPrefix, Language: e.Language, Kind: string(e.AnalysisKind),
			Mode: string(e.Mode), ProfileID: e.ProfileId, ImageDigest: e.ImageDigest,
		}
		if matches(f.Kind, row.Kind) && matches(f.Mode, row.Mode) && matches(f.ProfileID, row.ProfileID) && matches(f.ComponentID, row.ComponentID.String()) {
			out = append(out, row)
		}
	}
	return out
}

func matches(want, got string) bool { return want == "" || want == got }

// Store keeps the state of one job between invocations: the profile, the tool runs and their reports.
type Store struct {
	dir      string
	Scrubber Scrubber
}

func NewStore(dir string) *Store { return &Store{dir: dir} }

// Profile returns the saved profile of the job, or false when the job has not read it yet.
func (s *Store) Profile() (*ciapi.CiCheckProfile, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, _profileFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read saved profile: %w", err)
	}
	var p ciapi.CiCheckProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, false, fmt.Errorf("decode saved profile: %w", err)
	}
	return &p, true, nil
}

func (s *Store) SaveProfile(p *ciapi.CiCheckProfile) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	return s.write(_profileFile, data)
}

// AddRun records a tool run and copies its reports into the job state; Reports carry the package path of
// each copy, the hash and size are taken when the package is built.
func (s *Store) AddRun(run *envelope.ScanToolRun, reports []string) error {
	if !_runID.MatchString(run.Id) {
		return errors.New("tool run ID must be 1-64 letters, digits, '.', '_' or '-'")
	}
	name := path.Join(_runsDir, run.Id+".json")
	if _, err := os.Stat(filepath.Join(s.dir, name)); err == nil {
		return ErrRunExists
	}
	for i, local := range reports {
		target := path.Join(_reportsDir, run.Id, fmt.Sprintf("%d-%s", i+1, _unsafeChars.ReplaceAllString(filepath.Base(local), "_")))
		if err := s.copyIn(local, target); err != nil {
			return err
		}
		run.Reports[i].File.Path = target
	}
	data, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("encode tool run: %w", err)
	}
	return s.write(name, data)
}

// Runs returns the recorded tool runs ordered by ID.
func (s *Store) Runs() ([]envelope.ScanToolRun, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, _runsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list tool runs: %w", err)
	}
	runs := make([]envelope.ScanToolRun, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, _runsDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read tool run: %w", err)
		}
		var run envelope.ScanToolRun
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, fmt.Errorf("decode tool run %s: %w", e.Name(), err)
		}
		runs = append(runs, run)
	}
	slices.SortFunc(runs, func(a, b envelope.ScanToolRun) int { return strings.Compare(a.Id, b.Id) })
	return runs, nil
}

// ReadReport returns the stored copy of a report by its package path.
func (s *Store) ReadReport(packagePath string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(packagePath)))
	if err != nil {
		return nil, fmt.Errorf("read stored report: %w", err)
	}
	return data, nil
}

func (s *Store) copyIn(local, target string) error {
	src, err := os.Open(local) //nolint:gosec // the CI configuration names the report file
	if err != nil {
		return fmt.Errorf("open report: %w", err)
	}
	defer src.Close()
	dst := filepath.Join(s.dir, filepath.FromSlash(target))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // the path is built from a checked run ID
	if err != nil {
		return fmt.Errorf("create report copy: %w", err)
	}
	var input io.Reader = src
	if s.Scrubber != nil {
		data, err := io.ReadAll(src)
		if err != nil {
			out.Close()
			return errors.New("cannot read report for secret redaction")
		}
		data, err = s.Scrubber.Redact(data)
		if err != nil {
			out.Close()
			return err
		}
		input = bytes.NewReader(data)
	}
	if _, err := io.Copy(out, input); err != nil {
		out.Close()
		return fmt.Errorf("copy report: %w", err)
	}
	return out.Close()
}

func (s *Store) write(name string, data []byte) error {
	if s.Scrubber != nil {
		safe, err := s.Scrubber.Redact(data)
		if err != nil {
			return err
		}
		data = safe
	}
	target := filepath.Join(s.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("save %s: %w", name, err)
	}
	return nil
}
