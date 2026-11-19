package ci

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _producer = "craftpulse-ci"

// ErrNoRuns is returned when the job has recorded no tool run to pack.
var ErrNoRuns = errors.New("no tool runs recorded in this job: run a tool or import reports first")

// Scrubber removes the values of secrets from a report before it is packed (BR-017).
type Scrubber interface {
	Scrub(report *envelope.ScanReport, content []byte) ([]byte, error)
}

// KeepContent passes reports unchanged when the secrets profile is not active.
type KeepContent struct{}

func (KeepContent) Scrub(_ *envelope.ScanReport, content []byte) ([]byte, error) { return content, nil }

// Package is a checked package of the job ready for upload.
type Package struct {
	Data []byte
	Scan *envelope.ScanManifestV1
}

// Packer builds the canonical package of the job from its recorded tool runs.
type Packer struct {
	Job      *Job
	Store    *Store
	Scrubber Scrubber
	Version  string
	Now      func() time.Time
}

type packedFile struct {
	path, mediaType string
	content         []byte
}

// Build packs the runs and checks the package against the limits of the profile before it is sent; a
// package the intake would refuse is a refusal here.
func (p *Packer) Build(limits envelope.Limits) (*Package, error) {
	runs, err := p.Store.Runs()
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, ErrNoRuns
	}
	for i := range runs {
		if runs[i].ProfileId != nil && *runs[i].ProfileId == SecretsProfileID && p.Scrubber == nil {
			return nil, errors.New("secrets profile requires a fresh in-memory scrubber before upload")
		}
	}
	now := p.Now().UTC()
	var files []packedFile
	for i := range runs {
		for j := range runs[i].Reports {
			report := &runs[i].Reports[j]
			f, err := p.prepare(report)
			if err != nil {
				return nil, err
			}
			files = append(files, f)
		}
	}
	scan := &envelope.ScanManifestV1{SchemaVersion: "1", Tools: runs}
	if err := scrubMetadata(p.Scrubber, scan); err != nil {
		return nil, err
	}
	manifest := p.manifest(runs, now)
	if err := scrubMetadata(p.Scrubber, manifest); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w, err := envelope.NewWriter(&buf, manifest)
	if err != nil {
		return nil, err
	}
	if err := w.AddScanManifest(scan); err != nil {
		return nil, err
	}
	for _, f := range files {
		if _, err := w.AddFile(f.path, f.mediaType, f.content); err != nil {
			return nil, err
		}
	}
	for i := range runs {
		for j := range runs[i].Reports {
			rec, err := reportRecord(&runs[i], &runs[i].Reports[j], now)
			if err != nil {
				return nil, err
			}
			if err := scrubMetadata(p.Scrubber, rec); err != nil {
				return nil, err
			}
			if err := w.AddRecord(rec); err != nil {
				return nil, err
			}
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if verifier, ok := p.Scrubber.(interface{ VerifyPackage([]byte) error }); ok {
		if err := verifier.VerifyPackage(buf.Bytes()); err != nil {
			return nil, err
		}
	}
	if err := check(buf.Bytes(), limits); err != nil {
		return nil, err
	}
	return &Package{Data: buf.Bytes(), Scan: scan}, nil
}

func (p *Packer) prepare(report *envelope.ScanReport) (packedFile, error) {
	content, err := p.Store.ReadReport(report.File.Path)
	if err != nil {
		return packedFile{}, err
	}
	scrubber := p.Scrubber
	if scrubber == nil {
		scrubber = KeepContent{}
	}
	content, err = scrubber.Scrub(report, content)
	if err != nil {
		return packedFile{}, fmt.Errorf("remove secret values from %s: %w", report.File.Path, err)
	}
	sum := sha256.Sum256(content)
	report.File.Sha256, report.File.SizeBytes = hex.EncodeToString(sum[:]), len(content)
	return packedFile{path: report.File.Path, mediaType: report.File.MediaType, content: content}, nil
}

func (p *Packer) manifest(runs []envelope.ScanToolRun, now time.Time) *envelope.Manifest {
	m := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaIdCiRuns, SchemaVersion: "1",
		Source:    envelope.Source{Provider: envelope.SourceProviderGitlab, InstanceKey: p.Job.InstanceKey},
		Area:      envelope.AreaRef{Kind: envelope.AreaRefKindRepository, NativeId: p.Job.ProjectID, Key: optional(p.Job.ProjectPath)},
		Selection: envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
		Producer:  envelope.Producer{Name: _producer, Version: p.Version}, CreatedAt: now,
		Completeness: envelope.ManifestCompletenessComplete,
	}
	for i := range runs {
		switch runs[i].Completeness {
		case envelope.ScanToolRunCompletenessComplete:
		case envelope.ScanToolRunCompletenessPartial:
			m.Completeness = envelope.ManifestCompletenessPartial
		case envelope.ScanToolRunCompletenessUnknown:
			if m.Completeness == envelope.ManifestCompletenessComplete {
				m.Completeness = envelope.ManifestCompletenessUnknown
			}
		}
	}
	return m
}

// reportRecord is the report_file record of a report: native_ns is the CI attempt, or the SHA without it,
// native_id is the kind and the package path (TZ 12.6).
func reportRecord(run *envelope.ScanToolRun, report *envelope.ScanReport, now time.Time) (*envelope.Record, error) {
	exit, completeness := envelope.ReportFileV1ExitClass(run.ExitClass), envelope.ReportFileV1Completeness(run.Completeness)
	started, finished := run.StartedAt, run.FinishedAt
	payload := envelope.ReportFileV1{
		ReportKind: envelope.ReportFileV1ReportKind(report.ReportKind), ReportFormat: envelope.ReportFileV1ReportFormat(report.ReportFormat),
		Sha: run.Sha, ArtifactDigest: run.ArtifactDigest, Producer: run.Producer, ProducerVersion: run.ProducerVersion,
		ProfileId: run.ProfileId, RulesHash: run.RulesHash, ConfigHash: run.ConfigHash, DbVersion: run.DbVersion,
		StartedAt: &started, FinishedAt: &finished, ExitClass: &exit, Completeness: &completeness,
	}
	ns := ""
	if run.Sha != nil {
		ns = *run.Sha
	}
	if run.Ci != nil {
		payload.RunNativeId, payload.AttemptNativeId = &run.Ci.RunNativeId, run.Ci.AttemptNativeId
		if run.Ci.AttemptNativeId != nil {
			ns = *run.Ci.AttemptNativeId
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode report record: %w", err)
	}
	revision := report.File.Sha256
	return &envelope.Record{
		RecordClass: envelope.RecordRecordClassFile, RecordKind: envelope.RecordRecordKindReportFile, PayloadSchema: "report_file/1",
		NativeNs: ns, NativeId: string(report.ReportKind) + ":" + report.File.Path, SourceRevision: &revision,
		ObservedTime: &now, File: &report.File, Payload: body,
	}, nil
}

func check(data []byte, limits envelope.Limits) error {
	result := envelope.Check(bytes.NewReader(data), limits)
	if r := result.Envelope; r != nil {
		return &RefusedError{Code: r.Code, Detail: "checked before upload: " + r.Detail}
	}
	for _, rec := range result.Records {
		if rec.Rejection != nil {
			return &RefusedError{Code: rec.Rejection.Code, Detail: fmt.Sprintf("checked before upload: record %d: %s", rec.No, rec.Rejection.Detail)}
		}
	}
	return nil
}

// scrubMetadata covers provenance strings in persisted state and in the package.
func scrubMetadata(scrubber Scrubber, value any) error {
	if scrubber == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data, err = scrubber.Scrub(nil, data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return errors.New("cannot decode redacted package metadata")
	}
	return nil
}
