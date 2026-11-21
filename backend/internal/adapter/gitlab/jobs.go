package gitlab

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _maxPipelineJobs = 1000

var (
	_matrixSuffix      = regexp.MustCompile(`^(.+): \[(.*)\]$`)
	_parallelSuffix    = regexp.MustCompile(`^(.+) ([1-9]\d*)/[1-9]\d*$`)
	errJobHistoryLimit = errors.New("GitLab pipeline job history exceeded the page budget")
)

type ciJob struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	AllowFailure  *bool      `json:"allow_failure"`
	FailureReason string     `json:"failure_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	Duration      *float64   `json:"duration"`
	Runner        struct {
		Type string `json:"runner_type"`
	} `json:"runner"`
	Artifacts []struct {
		Type string `json:"file_type"`
	} `json:"artifacts"`
	ExpiresAt *time.Time `json:"artifacts_expire_at"`
	ErasedAt  *time.Time `json:"erased_at"`
	reports   reportTypes
}

func (s source) pipelineJobs(ctx context.Context, run pipeline) ([]ciJob, bool, error) {
	path := pipelinePath(run.ProjectID, run.ID) + "/jobs"
	query := url.Values{"include_retried": {"true"}}
	jobs := make([]ciJob, 0, _sourcePageSize)
	seen := make(map[int64]struct{})
	truncated := false
	for page := 1; page > 0; {
		next, err := readPage(ctx, s, "pipeline_jobs", path, query, page, _sourcePageSize, func(job ciJob) error {
			if len(jobs) == _maxPipelineJobs {
				return errJobHistoryLimit
			}
			if !job.valid() {
				truncated = true
				return nil
			}
			if _, duplicate := seen[job.ID]; duplicate {
				truncated = true
				return nil
			}
			seen[job.ID] = struct{}{}
			for _, artifact := range job.Artifacts {
				job.reports.add(artifact.Type)
			}
			job.Artifacts = nil
			jobs = append(jobs, job)
			return nil
		})
		switch {
		case err == nil:
			page = next
		case errors.Is(err, errJobHistoryLimit),
			hasCode(err, adaptersdk.AdapterErrorCodeNotFound), hasCode(err, adaptersdk.AdapterErrorCodeAuth),
			page > 1 && (hasCode(err, adaptersdk.AdapterErrorCodeUnavailable) || hasCode(err, adaptersdk.AdapterErrorCodeInvalidResponse)):
			truncated = true
			page = 0
		default:
			return nil, false, err
		}
	}
	slices.SortFunc(jobs, func(a, b ciJob) int {
		if created := a.CreatedAt.Compare(b.CreatedAt); created != 0 {
			return created
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return jobs, truncated, nil
}

func (j ciJob) valid() bool {
	return j.ID > 0 && j.Name != "" && utf8.RuneCountInString(j.Name) <= 255 &&
		j.Status != "" && len(j.Status) <= 64 && len(j.FailureReason) <= 64 &&
		j.AllowFailure != nil && !j.CreatedAt.IsZero() && len(j.Runner.Type) <= 64 &&
		(j.Duration == nil || *j.Duration >= 0 && *j.Duration < float64(math.MaxInt32))
}

type jobConfiguration struct {
	name, fingerprint string
}

func jobConfig(name string) jobConfiguration {
	key, config := name, ""
	if parts := _matrixSuffix.FindStringSubmatch(name); parts != nil {
		// GitLab renders the values in axis order and overwrites combinations with identical rendered names.
		key, config = parts[1], "matrix:"+parts[2]
	} else if parts := _parallelSuffix.FindStringSubmatch(name); parts != nil {
		key, config = parts[1], "parallel:"+parts[2]
	}
	sum := sha256.Sum256([]byte(config))
	return jobConfiguration{name: key, fingerprint: hex.EncodeToString(sum[:])}
}

func ciOutcome(status string) string {
	switch status {
	case "created", "waiting_for_resource", "preparing", "pending", "scheduled", "running":
		return "running"
	case "canceling", "canceled":
		return "canceled"
	case "manual", "skipped":
		return "skipped"
	case "success", "failed":
		return status
	default:
		return "unknown"
	}
}

func gitLabGlobalID(raw string, types ...string) string {
	for _, kind := range types {
		suffix, ok := strings.CutPrefix(raw, "gid://gitlab/"+kind+"/")
		if _, valid := numericID(suffix); ok && valid {
			return suffix
		}
	}
	return ""
}

func (s source) addPipelineAttempts(ctx context.Context, run pipeline, parent *string, jobs []ciJob, now time.Time, page *adaptersdk.Page) error {
	numbers := make(map[jobConfiguration]int)
	budget := reportBudget{bytes: _maxPageReportBytes, files: _maxPageReportFiles}
	for i := range jobs {
		job := &jobs[i]
		config := jobConfig(job.Name)
		numbers[config]++
		status, err := s.collectJobReports(ctx, run, job, now, page, &budget)
		if err != nil {
			return err
		}
		var duration *int
		if job.Duration != nil {
			seconds := int(*job.Duration)
			duration = &seconds
		}
		var artifactStatus *envelope.CiAttemptV2ArtifactStatus
		if status != "" {
			artifactStatus = &status
		}
		failureKind := optional(job.FailureReason)
		if job.FailureReason == "unknown_failure" {
			failureKind = nil
		}
		payload := envelope.CiAttemptV2{
			RunNativeId: id(run.ID), ParentRunNativeId: parent, JobKey: config.name, JobName: job.Name, ConfigFingerprint: config.fingerprint,
			AttemptNo: numbers[config], NativeStatus: job.Status, Outcome: envelope.CiAttemptV2Outcome(ciOutcome(job.Status)),
			AllowFailure: *job.AllowFailure, FailureKind: failureKind, FailureReason: optional(job.FailureReason),
			RunnerClass: optional(job.Runner.Type), CreatedAt: &job.CreatedAt, StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
			DurationS: duration, DurationSeconds: job.Duration, ArtifactStatus: artifactStatus,
		}
		if err := addCIRecord(page, "ci_attempt", "2", id(job.ID), now, payload); err != nil {
			return err
		}
	}
	return nil
}

func addCIRecord(page *adaptersdk.Page, kind, version, nativeID string, now time.Time, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	record := envelope.Record{
		RecordKind: envelope.RecordRecordKind(kind), PayloadSchema: kind + "/" + version,
		RecordClass: envelope.RecordRecordClassObject, NativeId: nativeID, ObservedTime: &now, Payload: payload,
	}
	revision, err := envelope.ContentHash(&record)
	if err != nil {
		return err
	}
	record.SourceRevision = &revision
	return page.AddRecord(&record)
}
