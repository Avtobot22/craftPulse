package gitlab

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_maxReportBytes     = 8 << 20
	_maxPageReportBytes = 16 << 20
	_maxPageReportFiles = 900
)

var (
	errReportRead = errors.New("GitLab report download interrupted")
	errReportSize = errors.New("GitLab report exceeds the size limit")
)

type reportTypes uint8

const (
	_reportJUnit reportTypes = 1 << iota
	_reportCobertura
	_reportJaCoCo
)

type reportType struct {
	mask     reportTypes
	fileType string
	kind     envelope.ReportFileV1ReportKind
	format   envelope.ReportFileV1ReportFormat
}

var _reportTypes = [...]reportType{
	{_reportJUnit, "junit", envelope.ReportFileV1ReportKindJunit, envelope.ReportFileV1ReportFormatJunitXml},
	{_reportCobertura, "cobertura", envelope.ReportFileV1ReportKindCoverage, envelope.ReportFileV1ReportFormatCobertura},
	{_reportJaCoCo, "jacoco", envelope.ReportFileV1ReportKindCoverage, envelope.ReportFileV1ReportFormatJacoco},
}

func (r *reportTypes) add(fileType string) {
	for _, report := range _reportTypes {
		if report.fileType == fileType {
			*r |= report.mask
			return
		}
	}
}

type reportBudget struct {
	bytes int64
	files int
}

func (s source) collectJobReports(ctx context.Context, run pipeline, job *ciJob, now time.Time, page *adaptersdk.Page, budget *reportBudget) (envelope.CiAttemptV2ArtifactStatus, error) {
	expired := job.ErasedAt != nil || job.ExpiresAt != nil && !now.Before(*job.ExpiresAt)
	var status envelope.CiAttemptV2ArtifactStatus
	if job.reports == 0 && expired {
		return envelope.CiAttemptV2ArtifactStatusExpired, nil
	}
	for _, report := range _reportTypes {
		if job.reports&report.mask == 0 {
			continue
		}
		ref, reason, err := s.fetchReport(ctx, run.ProjectID, job.ID, report, page, budget)
		if err != nil {
			return "", err
		}
		if reason != "" {
			if reason == envelope.CiAttemptV2ArtifactStatusUnavailable && expired {
				reason = envelope.CiAttemptV2ArtifactStatusExpired
			}
			if status == "" {
				status = reason
			}
			continue
		}
		if err := addReportFile(page, run, job, report, ref, now); err != nil {
			return "", err
		}
	}
	return status, nil
}

func (s source) fetchReport(ctx context.Context, projectID, jobID int64, report reportType, page *adaptersdk.Page, budget *reportBudget) (envelope.FileRef, envelope.CiAttemptV2ArtifactStatus, error) {
	limit := min(_maxReportBytes, budget.bytes)
	if budget.files == 0 || limit == 0 {
		return envelope.FileRef{}, envelope.CiAttemptV2ArtifactStatusSizeLimit, nil
	}
	path := "/api/v4/" + projectPath(projectID) + "/jobs/" + id(jobID) + "/artifacts"
	resp, err := s.requestURL(ctx, "job_report", path, url.Values{"file_type": {report.fileType}}, "application/octet-stream", adaptersdk.DefaultMaxBodyBytes)
	if err != nil {
		if ctx.Err() != nil || hasCode(err, adaptersdk.AdapterErrorCodeRateLimited) || hasCode(err, adaptersdk.AdapterErrorCodeTimeout) {
			return envelope.FileRef{}, "", err
		}
		return envelope.FileRef{}, envelope.CiAttemptV2ArtifactStatusUnavailable, nil
	}
	defer func() { _ = resp.Body.Close() }()

	r := bufio.NewReader(resp.Body)
	magic, err := r.Peek(2)
	if err != nil {
		return envelope.FileRef{}, envelope.CiAttemptV2ArtifactStatusUnavailable, nil
	}
	var content io.Reader = r
	if magic[0] == 0x1f && magic[1] == 0x8b {
		decoded, err := gzip.NewReader(r)
		if err != nil {
			return envelope.FileRef{}, envelope.CiAttemptV2ArtifactStatusUnavailable, nil
		}
		defer func() { _ = decoded.Close() }()
		content = decoded
	}
	name := "files/reports/" + id(jobID) + "/" + report.fileType + ".xml"
	bounded := &reportReader{reader: content, left: limit}
	ref, err := page.AddFile(name, "application/xml", bounded)
	budget.bytes -= limit - bounded.left
	switch {
	case err == nil:
		budget.files--
		return ref, "", nil
	case ctx.Err() != nil:
		return envelope.FileRef{}, "", adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "GitLab report download exceeded the deadline")
	case errors.Is(err, errReportSize), errors.Is(err, adaptersdk.ErrPageFull):
		return envelope.FileRef{}, envelope.CiAttemptV2ArtifactStatusSizeLimit, nil
	case errors.Is(err, errReportRead):
		return envelope.FileRef{}, envelope.CiAttemptV2ArtifactStatusUnavailable, nil
	default:
		return envelope.FileRef{}, "", err
	}
}

type reportReader struct {
	reader io.Reader
	left   int64
}

func (r *reportReader) Read(p []byte) (int, error) {
	if r.left == 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, errReportSize
		}
		return 0, reportReadError(err)
	}
	n, err := r.reader.Read(p[:min(int64(len(p)), r.left)])
	r.left -= int64(n)
	return n, reportReadError(err)
}

func reportReadError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	return fmt.Errorf("%w: %w", errReportRead, err)
}

func addReportFile(page *adaptersdk.Page, run pipeline, job *ciJob, report reportType, ref envelope.FileRef, now time.Time) error {
	attemptID, runID := id(job.ID), id(run.ID)
	payload, err := json.Marshal(envelope.ReportFileV1{
		ReportKind: report.kind, ReportFormat: report.format, Sha: &run.SHA,
		RunNativeId: &runID, AttemptNativeId: &attemptID, Producer: "unknown", ProducerVersion: "unknown",
		StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
	})
	if err != nil {
		return err
	}
	rec := envelope.Record{
		RecordClass: envelope.RecordRecordClassFile, RecordKind: envelope.RecordRecordKindReportFile, PayloadSchema: "report_file/1",
		NativeNs: attemptID, NativeId: string(report.kind) + ":" + ref.Path, SourceRevision: &ref.Sha256,
		ObservedTime: &now, File: &ref, Payload: payload,
	}
	return page.AddRecord(&rec)
}
