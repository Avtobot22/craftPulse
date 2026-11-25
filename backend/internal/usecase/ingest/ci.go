package ingest

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func (u *UseCase) ciScope(ctx context.Context, rc entity.Receipt, m *envelope.Manifest) (entity.ID, bool, error) {
	inst, err := u.d.Connect.EnsureInstance(ctx, rc.OrganizationID, entity.Provider(m.Source.Provider), m.Source.InstanceKey)
	if err != nil {
		return entity.ID{}, false, err
	}
	kind := entity.ScopeRepository
	if m.Source.Provider == envelope.SourceProviderJenkins {
		kind = entity.ScopeCIProject
	}
	if string(m.Area.Kind) != string(kind) {
		return inst.ID, false, nil
	}
	sources, err := u.d.Projects.ProjectSources(ctx, rc.OrganizationID, *rc.ProjectID, kind)
	if err != nil {
		return entity.ID{}, false, err
	}
	allowed := slices.ContainsFunc(sources, func(r entity.AreaSource) bool {
		return r.InstanceID == inst.ID && r.NativeScopeID == m.Area.NativeId
	})
	return inst.ID, allowed, nil
}

// jenkinsAreaMatches binds both manifests to the Jenkins job authorized by the token.
func jenkinsAreaMatches(res *envelope.Result) bool {
	job := res.Manifest.Area.NativeId
	execution := res.ExecutionManifest
	attempts := map[string]int{}
	if execution != nil {
		if execution.Job != job {
			return false
		}
		for _, step := range execution.Steps {
			key := envelope.JenkinsAttemptKey(job, execution.BuildNumber, step.Stage, step.Name, step.ConfigurationHash, step.Attempt)
			attempts[key] = step.Attempt
		}
	}
	if res.ScanManifest != nil {
		for _, tool := range res.ScanManifest.Tools {
			c := tool.Ci
			if c == nil || c.Provider != envelope.ScanCiCoordinatesProviderJenkins || c.ProjectNativeId != job ||
				c.JobNativeId == nil || *c.JobNativeId != job || (execution != nil && c.RunNativeId != strconv.Itoa(execution.BuildNumber)) {
				return false
			}
			if c.AttemptNativeId != nil && (c.AttemptNo == nil || attempts[*c.AttemptNativeId] != *c.AttemptNo) {
				return false
			}
		}
	}
	return true
}

func restrictCI(recs []entity.IngestRecord, pkg *envelope.Package) error {
	for i := range recs {
		r := &recs[i]
		if r.Outcome == entity.RecordRejected {
			continue
		}
		if (pkg.Manifest.Source.Provider == envelope.SourceProviderJenkins && r.Kind != "report_file") ||
			!slices.Contains([]string{"ci_run", "ci_attempt", "report_file", "finding"}, r.Kind) {
			r.Outcome, r.ReasonCode = entity.RecordRejected, entity.RecordOutsideChannel
			continue
		}
		if r.Kind == "ci_run" {
			record, ok, err := pkg.Record(r.No)
			if err != nil {
				return err
			}
			if !ok {
				return errorf("cleaned package has no record %d", r.No)
			}
			var payload envelope.CiRunV1
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				return err
			}
			if payload.ProjectNativeId != pkg.Manifest.Area.NativeId {
				r.Outcome, r.ReasonCode = entity.RecordRejected, entity.RecordOutsideScope
			}
		}
	}
	return nil
}

func (u *UseCase) firstReceiptOfHash(ctx context.Context, rc entity.Receipt, hash []byte) (entity.ID, bool, error) {
	if rc.Channel == entity.ChannelCI {
		return u.d.Repo.FirstCIReceiptOfHash(ctx, rc.OrganizationID, *rc.ProjectID, rc.ID, hash)
	}
	return u.d.Repo.FirstReceiptOfHash(ctx, rc.OrganizationID, hash)
}

func ciReason(p *entity.PreparedPackage) string {
	if p.Receipt.ReasonCode != "" {
		return p.Receipt.ReasonCode
	}
	reason := ""
	for i := range p.Records {
		rec := &p.Records[i]
		if rec.Outcome != entity.RecordRejected {
			continue
		}
		if reason != "" && reason != rec.ReasonCode {
			return "multiple_record_rejections"
		}
		reason = rec.ReasonCode
	}
	return reason
}
