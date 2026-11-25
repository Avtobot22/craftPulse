package normalize

import (
	"context"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type ciReceiptTarget struct {
	run     entity.SourceKey
	mr, sha string
}

func (in *input) receiptTarget(pkg *envelope.Package) *ciReceiptTarget {
	if pkg.ScanManifest != nil {
		for i := range pkg.ScanManifest.Tools {
			tool := &pkg.ScanManifest.Tools[i]
			c := tool.Ci
			if c != nil && string(c.Provider) == in.provider && c.ProjectNativeId == in.area.NativeID {
				return &ciReceiptTarget{in.runKey(in.area.NativeID, c.RunNativeId), deref(c.MrNativeId), deref(tool.Sha)}
			}
		}
	}
	if m := pkg.ExecutionManifest; m != nil {
		return &ciReceiptTarget{in.runKey(m.Job, strconv.Itoa(m.BuildNumber)), "", m.Sha}
	}
	return nil
}

func (w *writer) recordCIResult(ctx context.Context, in *input) error {
	if !in.ciReceipt {
		return nil
	}
	target := in.ciResult
	if target == nil {
		switch {
		case len(in.ciRuns) > 0:
			c := in.ciRuns[0]
			target = &ciReceiptTarget{c.key, c.mr, c.version.SHA}
		case len(in.ciAttempts) > 0:
			target = &ciReceiptTarget{run: in.ciAttempts[0].run}
		default:
			return nil
		}
	}
	run := w.objects[target.run]
	area := w.objects[in.area].ID
	if run.DeletedAt != nil || (run.CurrentAreaID != nil && *run.CurrentAreaID != area) {
		return nil
	}
	versions, err := w.u.d.Repo.CurrentCIRunVersions(ctx, w.org, []entity.ID{run.ID})
	if err != nil {
		return err
	}
	var mr *entity.ID
	if len(versions) > 0 {
		if versions[0].AreaID != area {
			return nil
		}
		if target.sha == "" || versions[0].SHA == target.sha {
			mr = versions[0].MRID
		}
	}
	if mr == nil && target.mr != "" && target.sha != "" {
		found, err := w.u.d.Repo.MatchRunMRs(ctx, w.org, in.instance, []entity.RunMRMatch{
			{MRNativeID: target.mr, AreaID: area, SHA: target.sha},
		})
		if err != nil {
			return err
		}
		if id, ok := found[0]; ok {
			mr = &id
		}
	}
	return w.u.d.Ingest.RecordCIResult(ctx, w.org, in.receipt, &run.ID, mr, true)
}
