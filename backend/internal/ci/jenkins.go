package ci

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const executionLimit = 1 << 20

// JenkinsStep is one completed invocation in a Jenkins stage or matrix cell.
type JenkinsStep struct {
	Stage, Name, Agent string
	Axes               map[string]string
	Attempt            int
	Outcome            string
	Started, Finished  time.Time
}

func jenkinsManifest(getenv func(string) string) (envelope.JenkinsExecutionManifestV1, error) {
	m := envelope.JenkinsExecutionManifestV1{
		SchemaVersion: "1", Job: getenv("JOB_NAME"), BuildUrl: getenv("BUILD_URL"), Sha: HeadSHA(getenv),
	}
	m.BuildNumber, _ = strconv.Atoi(getenv("BUILD_NUMBER"))
	if m.Job == "" || m.BuildNumber < 1 || m.BuildUrl == "" || !_sha.MatchString(m.Sha) {
		return m, errors.New("JOB_NAME, BUILD_NUMBER, BUILD_URL and GIT_COMMIT must identify the Jenkins build")
	}
	m.DatasetRunId = fmt.Sprintf("%x", sha256.Sum256([]byte(m.BuildUrl)))
	return m, nil
}

// Record writes an attempt atomically and returns its ID for the scan manifest.
func (s *JenkinsStep) Record(getenv func(string) string, dir string) (string, error) {
	m, err := jenkinsManifest(getenv)
	if err != nil {
		return "", err
	}
	hash := envelope.JenkinsConfigurationHash(s.Axes)
	datasetAttemptID := envelope.JenkinsAttemptKey(m.Job, m.BuildNumber, s.Stage, s.Name, hash, s.Attempt)
	// The generated schema declares the step as an anonymous struct.
	m.Steps = slices.Grow(m.Steps, 1)[:1]
	step := &m.Steps[0]
	step.Stage, step.Name, step.AgentLabel = s.Stage, s.Name, s.Agent
	step.Axes, step.Attempt = s.Axes, s.Attempt
	step.Outcome = envelope.JenkinsExecutionManifestV1StepsOutcome(s.Outcome)
	step.StartedAt, step.FinishedAt = s.Started.UTC(), s.Finished.UTC()
	step.ConfigurationHash, step.DatasetAttemptId = hash, datasetAttemptID
	step.Reports = []executionReport{}
	data, err := marshalExecution(&m)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "attempt-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return "", err
	}
	return datasetAttemptID, os.Link(f.Name(), filepath.Join(dir, datasetAttemptID+".json"))
}

type executionReport = struct {
	Format    envelope.JenkinsExecutionManifestV1StepsReportsFormat `json:"format"`
	Path      string                                                `json:"path"`
	Sha256    string                                                `json:"sha256"`
	SourceSha string                                                `json:"source_sha"`
}

func marshalExecution(m *envelope.JenkinsExecutionManifestV1) ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if _, err := validateExecution(data); err != nil {
		return nil, err
	}
	return data, nil
}

// FinishJenkinsExecution collects attempts written by parallel branches and retries into one artifact.
func FinishJenkinsExecution(getenv func(string) string, dir, output string) error {
	m, err := jenkinsManifest(getenv)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		attempt, err := ReadJenkinsExecution(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		if attempt.Job != m.Job || attempt.BuildNumber != m.BuildNumber || attempt.BuildUrl != m.BuildUrl || attempt.Sha != m.Sha {
			return errors.New("recorded execution attempt belongs to another Jenkins build")
		}
		m.Steps = append(m.Steps, attempt.Steps...)
	}
	data, err := marshalExecution(&m)
	if err != nil {
		return err
	}
	return os.WriteFile(output, data, 0o600)
}

// ReadJenkinsExecution loads and validates a Jenkins execution manifest.
func ReadJenkinsExecution(path string) (*envelope.JenkinsExecutionManifestV1, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, executionLimit+1))
	if err != nil {
		return nil, err
	}
	return validateExecution(data)
}

func validateExecution(data []byte) (*envelope.JenkinsExecutionManifestV1, error) {
	if len(data) > executionLimit {
		return nil, errors.New("execution manifest exceeds the Jenkins size limit of 1 MiB")
	}
	m, rejection := envelope.ValidateExecutionManifest(data)
	if rejection != nil {
		return nil, fmt.Errorf("%s: %s", rejection.Code, rejection.Detail)
	}
	return m, nil
}
