package normalize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_recordExecutionManifest = "execution_manifest"
	_formatJenkinsExecution  = "jenkins_execution_manifest"
	// _buildJobKey is the one logical check of a Jenkins build without a manifest: the build itself. A step of a
	// manifest is named stage/step, so the key never meets one.
	_buildJobKey = "build"
)

// _stepOutcomes maps the outcomes of steps of an execution manifest; the outcome itself stays the native status.
var _stepOutcomes = map[envelope.JenkinsExecutionManifestV1StepsOutcome]entity.CIOutcome{
	envelope.JenkinsExecutionManifestV1StepsOutcomeSuccess:  entity.CISuccess,
	envelope.JenkinsExecutionManifestV1StepsOutcomeFailure:  entity.CIFailed,
	envelope.JenkinsExecutionManifestV1StepsOutcomeTimeout:  entity.CIFailed,
	envelope.JenkinsExecutionManifestV1StepsOutcomeCanceled: entity.CICanceled,
}

// manifestCandidate is an execution manifest of a Jenkins build: a file record an adapter read from the artifacts
// of the build, or the execution-manifest.json of a package of the CI step, whose recordNo is zero. Its attempts are
// decided once the build is known, since a manifest of another build or SHA is rejected.
type manifestCandidate struct {
	ref      entity.RecordRef
	observed time.Time
	file     string
	parser   *reportParser
	declared *envelope.ExecutionManifestFileV1
	manifest *envelope.JenkinsExecutionManifestV1
	run      entity.SourceKey
	attempts []*ciAttemptCandidate
	rejected string
}

func (in *input) manifestRecord(rec *envelope.Record, b baseFact) (*manifestCandidate, error) {
	var p envelope.ExecutionManifestFileV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode execution manifest %s: %w", rec.NativeId, err)
	}
	if rec.File == nil {
		return nil, errorf("execution manifest %s has no file", rec.NativeId)
	}
	c := &manifestCandidate{ref: b.ref, observed: b.observed, file: rec.File.Path, declared: &p, run: in.runKey(p.Job, strconv.Itoa(p.BuildNumber))}
	c.parser, _ = parserOf(_formatJenkinsExecution)
	if rec.NativeNs != p.Job || rec.NativeId != strconv.Itoa(p.BuildNumber) {
		c.rejected = entity.RecordManifestMismatch
	}
	return c, nil
}

// jenkinsBuilds adds the execution manifest of a package of the CI step to the first part and the attempt that stands for
// every finished Jenkins build of the part; whether the build gets it is decided with the current facts.
func (in *input) jenkinsBuilds(rc entity.Receipt, pkg *envelope.Package, part []entity.IngestRecord, first bool) error {
	if in.provider != _providerJenkins {
		return nil
	}
	if m := pkg.ExecutionManifest; m != nil && first && len(part) > 0 {
		c := &manifestCandidate{
			ref: entity.RecordRef{ReceiptID: rc.ID, RecordNo: part[0].No}, observed: rc.ReceivedAt, manifest: m,
			run: in.runKey(m.Job, strconv.Itoa(m.BuildNumber)),
		}
		if err := in.stepAttempts(c); err != nil {
			return err
		}
		in.manifests = append(in.manifests, c)
	}
	in.buildAttempts = map[entity.SourceKey]*ciAttemptCandidate{}
	for _, r := range in.ciRuns {
		if r.version.FinishedAt == nil || r.version.Outcome == entity.CIRunning {
			continue
		}
		v := r.version
		a := entity.CIAttemptVersion{
			Record: v.Record, Revision: v.Revision, ObservedAt: v.ObservedAt, JobKey: _buildJobKey, ConfigFingerprint: buildFingerprint(),
			AttemptNo: 1, NativeStatus: v.NativeStatus, Outcome: v.Outcome, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt,
		}
		if v.StartedAt != nil {
			d := int64(v.FinishedAt.Sub(*v.StartedAt) / time.Second)
			a.DurationS = &d
		}
		in.buildAttempts[r.key] = &ciAttemptCandidate{key: buildAttemptKey(r.key), run: r.key, version: a}
	}
	return nil
}

func buildFingerprint() []byte {
	sum := sha256.Sum256([]byte(_buildJobKey))
	return sum[:]
}

// buildAttemptKey is the identity of the attempt that stands for a whole build: within the job, by the build number.
func buildAttemptKey(run entity.SourceKey) entity.SourceKey {
	return entity.SourceKey{InstanceID: run.InstanceID, Kind: entity.ObjectCIAttempt, NativeNS: run.NativeNS, NativeID: run.NativeID + "/" + _buildJobKey}
}

// stepAttempts makes an attempt of every step of a manifest. The identity of an attempt is the key of the envelope
// (job, build, stage, step, configuration and attempt number), and its revision is the hash of the step, so the
// same manifest read by the adapter and sent by the CI step gives the same versions.
func (in *input) stepAttempts(c *manifestCandidate) error {
	m := c.manifest
	for i := range m.Steps {
		s := &m.Steps[i]
		fingerprint, err := hex.DecodeString(s.ConfigurationHash)
		if err != nil {
			return errorf("decode configuration hash of build %s #%d: %w", m.Job, m.BuildNumber, err)
		}
		content, err := json.Marshal(struct {
			Stage, Name, ConfigurationHash, Outcome, AgentLabel string
			Axes                                                map[string]string
			Attempt                                             int
			StartedAt, FinishedAt                               time.Time
		}{s.Stage, s.Name, s.ConfigurationHash, string(s.Outcome), s.AgentLabel, s.Axes, s.Attempt, s.StartedAt, s.FinishedAt})
		if err != nil {
			return errorf("encode step of build %s #%d: %w", m.Job, m.BuildNumber, err)
		}
		sum := sha256.Sum256(content)
		started, finished := s.StartedAt, s.FinishedAt
		duration := int64(finished.Sub(started) / time.Second)
		outcome, ok := _stepOutcomes[s.Outcome]
		if !ok {
			outcome = entity.CIUnknown
		}
		id := envelope.JenkinsAttemptKey(m.Job, m.BuildNumber, s.Stage, s.Name, s.ConfigurationHash, s.Attempt)
		c.attempts = append(c.attempts, &ciAttemptCandidate{
			key: in.ciKey(entity.ObjectCIAttempt, id), run: c.run,
			version: entity.CIAttemptVersion{
				Record: c.ref, Revision: entity.Revision{Source: hex.EncodeToString(sum[:]), Hash: sum[:]}, ObservedAt: c.observed,
				JobKey: s.Stage + "/" + s.Name, ConfigFingerprint: fingerprint, AttemptNo: s.Attempt, NativeStatus: string(s.Outcome),
				Outcome: outcome, RunnerClass: s.AgentLabel, StartedAt: &started, FinishedAt: &finished, DurationS: &duration,
			},
		})
	}
	return nil
}

// parseJenkinsExecution reads an execution manifest by the schema of the envelope: a file of an adapter has not
// been checked at intake as the one of the CI step was.
func parseJenkinsExecution(r io.Reader, _ string) (*parsedReport, error) {
	data, err := io.ReadAll(io.LimitReader(r, _maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > _maxManifestBytes {
		return nil, &errReport{reason: envelope.ReasonInvalidExecutionManifest, detail: "larger than 1 MiB"}
	}
	m, rejection := envelope.ValidateExecutionManifest(data)
	if rejection != nil {
		return nil, &errReport{reason: rejection.Code, detail: rejection.Detail}
	}
	return &parsedReport{formatVersion: string(m.SchemaVersion), execution: m}, nil
}

const _maxManifestBytes = 1 << 20

// readManifest takes the parsed file of a manifest record, or the reason it was not read, and checks that it
// describes the build its record names.
func (in *input) readManifest(c *manifestCandidate, parsed *parsedReport, reason string) error {
	switch {
	case c.rejected != "":
		return nil
	case reason != "":
		c.rejected = reason
		return nil
	}
	m := parsed.execution
	if m.Job != c.declared.Job || m.BuildNumber != c.declared.BuildNumber || m.Sha != c.declared.Sha {
		c.rejected = entity.RecordManifestMismatch
		return nil
	}
	c.manifest = m
	return in.stepAttempts(c)
}

// decideBuilds decides the attempts of the Jenkins builds of a part before the CI facts are written. A manifest whose SHA
// differs from its build is rejected; a build with a manifest has a complete history, and a build marked cut or
// with a rejected manifest gets the attempt that stands for it and keeps unknown inner retries (FR-039 A3). An
// attempt that stood for a build gives way once its manifest comes.
func (w *writer) decideBuilds(ctx context.Context, in *input) error {
	if in.provider != _providerJenkins {
		return nil
	}
	parts := map[entity.SourceKey]*ciRunCandidate{}
	for _, r := range in.ciRuns {
		parts[r.key] = r
	}
	var ids []entity.ID
	for _, m := range in.manifests {
		ids = append(ids, w.objects[m.run].ID)
	}
	for _, r := range in.ciRuns {
		ids = append(ids, w.objects[r.key].ID)
	}
	current, err := w.u.d.Repo.CurrentCIRunVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	shas := make(map[entity.ID]string, len(current))
	for i := range current {
		shas[current[i].RunID] = current[i].SHA
	}
	reasons := map[int]string{}
	withManifest, withoutManifest := map[entity.SourceKey]bool{}, map[entity.SourceKey]bool{}
	for _, m := range in.manifests {
		sha := shas[w.objects[m.run].ID]
		if r, ok := parts[m.run]; ok {
			sha = r.version.SHA
		}
		if m.rejected == "" && sha != "" && sha != m.manifest.Sha {
			m.rejected = entity.RecordManifestMismatch
		}
		if m.rejected != "" {
			if m.declared != nil {
				reasons[m.ref.RecordNo] = m.rejected
			}
			withoutManifest[m.run] = true
			continue
		}
		withManifest[m.run] = true
		in.ciAttempts = append(in.ciAttempts, m.attempts...)
	}
	if err := w.u.d.Ingest.RejectRecords(ctx, w.org, in.receipt, reasons); err != nil {
		return err
	}
	runIDs := make([]entity.ID, 0, len(in.ciRuns))
	for _, r := range in.ciRuns {
		runIDs = append(runIDs, w.objects[r.key].ID)
	}
	known, err := w.u.d.Repo.RunsWithAttempts(ctx, w.org, runIDs, _buildJobKey)
	if err != nil {
		return err
	}
	for _, r := range in.ciRuns {
		switch {
		case withManifest[r.key] || known[w.objects[r.key].ID]:
			r.version.AttemptHistory = entity.AttemptHistoryComplete
		case withoutManifest[r.key] || r.version.AttemptHistory == entity.AttemptHistoryTruncated:
			r.version.AttemptHistory = entity.AttemptHistoryTruncated
			if a := in.buildAttempts[r.key]; a != nil {
				in.ciAttempts = append(in.ciAttempts, a)
			}
		}
	}
	return w.replaceBuildAttempts(ctx, withManifest)
}

// replaceBuildAttempts ends the attempts that stood for builds whose manifest came: the steps replace them.
func (w *writer) replaceBuildAttempts(ctx context.Context, runs map[entity.SourceKey]bool) error {
	if len(runs) == 0 {
		return nil
	}
	ids := make([]entity.ID, 0, len(runs))
	byID := make(map[entity.ID]entity.SourceObject, len(runs))
	for run := range runs {
		ids = append(ids, w.objects[buildAttemptKey(run)].ID)
		o := w.objects[run]
		byID[o.ID] = o
	}
	current, err := w.u.d.Repo.CurrentCIAttemptVersions(ctx, w.org, ids)
	if err != nil {
		return err
	}
	ended := make([]entity.ID, 0, len(current))
	for i := range current {
		a := &current[i]
		ended = append(ended, a.ID)
		if run := byID[a.RunID]; run.CurrentAreaID != nil {
			w.changes.add(*run.CurrentAreaID, _factCIAttempt, changedAt(a.StartedAt, a.FinishedAt))
		}
	}
	return w.u.d.Repo.SupersedeCIAttemptVersions(ctx, w.org, ended, w.seq, w.now)
}
