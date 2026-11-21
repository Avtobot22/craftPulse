package normalize

import (
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_providerGitLab  = "gitlab"
	_providerJenkins = "jenkins"
	// _gitlabUnknownFailure is the failure reason GitLab gives when it does not know the cause.
	_gitlabUnknownFailure = "unknown_failure"
)

// _gitlabOutcomes maps the statuses of GitLab pipelines and jobs to outcomes; a status missing here is unknown.
// A manual job waits for a person and has not run, so it is skipped until it is played.
var _gitlabOutcomes = map[string]entity.CIOutcome{
	"created":              entity.CIRunning,
	"waiting_for_resource": entity.CIRunning,
	"preparing":            entity.CIRunning,
	"pending":              entity.CIRunning,
	"scheduled":            entity.CIRunning,
	"running":              entity.CIRunning,
	"success":              entity.CISuccess,
	"failed":               entity.CIFailed,
	"canceling":            entity.CICanceled,
	"canceled":             entity.CICanceled,
	"skipped":              entity.CISkipped,
	"manual":               entity.CISkipped,
}

// _jenkinsOutcomes maps the results of Jenkins builds to outcomes; the result itself stays as the native status, so
// calc tells UNSTABLE from FAILURE. A build in progress has no result yet.
var _jenkinsOutcomes = map[string]entity.CIOutcome{
	"":          entity.CIRunning,
	"SUCCESS":   entity.CISuccess,
	"FAILURE":   entity.CIFailed,
	"UNSTABLE":  entity.CIFailed,
	"ABORTED":   entity.CICanceled,
	"NOT_BUILT": entity.CISkipped,
}

type ciRunCandidate struct {
	key, area entity.SourceKey
	parent    *entity.SourceKey
	mr        string
	version   entity.CIRunVersion
}

type ciAttemptCandidate struct {
	key, run entity.SourceKey
	version  entity.CIAttemptVersion
}

// truncatedRuns are the runs whose attempt history the package marks as cut.
func truncatedRuns(sel envelope.Selection) map[string]bool {
	out := map[string]bool{}
	for _, t := range sel.Truncated {
		if string(t.RecordKind) == string(entity.ObjectCIRun) {
			out[t.NativeId] = true
		}
	}
	return out
}

func (in *input) ciKey(kind entity.ObjectKind, id string) entity.SourceKey {
	return entity.SourceKey{InstanceID: in.instance, Kind: kind, NativeID: id}
}

// runKey is the identity of a CI run: a Jenkins build is unique within its job only (TZ 12.4).
func (in *input) runKey(job, id string) entity.SourceKey {
	k := in.ciKey(entity.ObjectCIRun, id)
	if in.provider == _providerJenkins {
		k.NativeNS = job
	}
	return k
}

func (in *input) ciRun(rec *envelope.Record, b baseFact, areaKind entity.ObjectKind) (*ciRunCandidate, error) {
	var p envelope.CiRunV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode CI run %s: %w", rec.NativeId, err)
	}
	if p.StartedAt != nil && p.FinishedAt != nil && p.FinishedAt.Before(*p.StartedAt) {
		return nil, errorf("CI run %s finished before it started", rec.NativeId)
	}
	history := entity.AttemptHistoryComplete
	if in.truncated[rec.NativeId] {
		history = entity.AttemptHistoryTruncated
	}
	c := &ciRunCandidate{
		key: in.runKey(rec.NativeNs, rec.NativeId), area: entity.SourceKey{InstanceID: in.instance, Kind: areaKind, NativeID: p.ProjectNativeId},
		mr: deref(p.MrNativeId),
		version: entity.CIRunVersion{
			Record: b.ref, Revision: b.revision, ObservedAt: b.observed, SourceUpdatedAt: p.UpdatedAt,
			RefName: p.RefName, SHA: p.Sha, DefinitionVersion: deref(p.DefinitionVersion), NativeStatus: p.NativeStatus,
			Outcome: in.outcome(p.NativeStatus, entity.CIOutcome(p.Outcome)), StartedAt: p.StartedAt, FinishedAt: p.FinishedAt,
			AttemptHistory: history,
		},
	}
	if parent := deref(p.ParentRunNativeId); parent != "" && parent != rec.NativeId {
		k := in.runKey(rec.NativeNs, parent)
		c.parent = &k
	}
	return c, nil
}

func (in *input) ciAttempt(rec *envelope.Record, b baseFact) (*ciAttemptCandidate, error) {
	var p envelope.CiAttemptV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode CI attempt %s: %w", rec.NativeId, err)
	}
	if p.StartedAt != nil && p.FinishedAt != nil && p.FinishedAt.Before(*p.StartedAt) {
		return nil, errorf("CI attempt %s finished before it started", rec.NativeId)
	}
	fingerprint, err := hex.DecodeString(p.ConfigFingerprint)
	if err != nil {
		return nil, errorf("decode config fingerprint of CI attempt %s: %w", rec.NativeId, err)
	}
	failure := deref(p.FailureKind)
	if in.provider == _providerGitLab && failure == _gitlabUnknownFailure {
		failure = ""
	}
	return &ciAttemptCandidate{
		key: in.ciKey(entity.ObjectCIAttempt, rec.NativeId), run: in.ciKey(entity.ObjectCIRun, p.RunNativeId),
		version: entity.CIAttemptVersion{
			Record: b.ref, Revision: b.revision, ObservedAt: b.observed, JobKey: p.JobKey, ConfigFingerprint: fingerprint,
			AttemptNo: p.AttemptNo, NativeStatus: p.NativeStatus, Outcome: in.outcome(p.NativeStatus, entity.CIOutcome(p.Outcome)),
			AllowFailure: p.AllowFailure, FailureKind: failure, RunnerClass: deref(p.RunnerClass),
			StartedAt: p.StartedAt, FinishedAt: p.FinishedAt, DurationS: duration(p.DurationS),
		},
	}, nil
}

// outcome takes the outcome of GitLab and Jenkins from the native status by their tables; other sources give it
// themselves.
func (in *input) outcome(native string, given entity.CIOutcome) entity.CIOutcome {
	table := map[string]map[string]entity.CIOutcome{_providerGitLab: _gitlabOutcomes, _providerJenkins: _jenkinsOutcomes}[in.provider]
	if table == nil {
		return given
	}
	if o, ok := table[native]; ok {
		return o
	}
	return entity.CIUnknown
}

// olderAttempt tells whether a state of an attempt is behind the current one: a finished attempt is not
// replaced by a state without its end.
func olderAttempt(v, than entity.CIAttemptVersion) bool {
	return than.FinishedAt != nil && v.FinishedAt == nil
}

func duration(s *int) *int64 {
	if s == nil {
		return nil
	}
	d := int64(*s)
	return &d
}

// changedAt is the time a run or an attempt counts in windows from: its start, or its end without one.
func changedAt(started, finished *time.Time) *time.Time {
	if started != nil {
		return started
	}
	return finished
}
