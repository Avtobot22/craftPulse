package ci

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/ci/ciapi"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// The job log is read line by line in a narrow terminal: one fact per line, plain words, no colours, and
// SHA and URL are never shortened (UI-51).

func PrintJob(w io.Writer, version string, j *Job, p *ciapi.CiCheckProfile) {
	line := fmt.Sprintf("craftpulse-ci %s: project %s, pipeline %s, job %s", version, j.ProjectID, j.PipelineID, j.JobID)
	if j.MergeRequestID != "" {
		line += ", merge request " + j.MergeRequestID
	}
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "sha: "+j.SHA)
	if p != nil {
		fmt.Fprintf(w, "check profile: version %d\n", p.ProfileVersionNo)
	}
}

func PrintEntries(w io.Writer, rows []Entry) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "profile rows for this job: none")
		return
	}
	for _, r := range rows {
		digest := "-"
		if r.ImageDigest != nil {
			digest = *r.ImageDigest
		}
		fmt.Fprintf(w, "profile row: %s %s %s component %s path %q language %q image %s\n",
			r.Kind, r.Mode, r.ProfileID, r.ComponentID, r.PathPrefix, r.Language, digest)
	}
}

func PrintRun(w io.Writer, run *envelope.ScanToolRun) {
	fmt.Fprintf(w, "tool run %s: %s %s by %s %s, %s, %s, %d report(s)\n", run.Id, run.ReportKind, run.Mode,
		run.Producer, run.ProducerVersion, run.ExitClass, run.Completeness, len(run.Reports))
}

// PrintPackage lists the tool runs of the package and the expected kinds of the profile it does not bring.
func PrintPackage(w io.Writer, pkg *Package, p *ciapi.CiCheckProfile) {
	kinds := map[string]bool{}
	reports := 0
	for i := range pkg.Scan.Tools {
		PrintRun(w, &pkg.Scan.Tools[i])
		if len(pkg.Scan.Tools[i].Reports) > 0 {
			kinds[string(pkg.Scan.Tools[i].ReportKind)] = true
		}
		reports += len(pkg.Scan.Tools[i].Reports)
	}
	var missing []string
	for _, k := range p.ExpectedReports {
		if !kinds[string(k)] && !slices.Contains(missing, string(k)) {
			missing = append(missing, string(k))
		}
	}
	if len(missing) > 0 {
		fmt.Fprintln(w, "expected reports not in this package: "+strings.Join(missing, ", "))
	}
	fmt.Fprintf(w, "package: %d tool run(s), %d report(s), %d bytes\n", len(pkg.Scan.Tools), reports, len(pkg.Data))
}

func PrintAccepted(w io.Writer, a *ciapi.ReceiptAccepted) {
	line := fmt.Sprintf("upload: receipt %s, %s", a.ReceiptId, a.Status)
	if a.DuplicateOf != nil {
		line += ", same package as receipt " + a.DuplicateOf.String()
	}
	fmt.Fprintln(w, line)
}

// PrintReceipt prints the result of the intake. A clean intake says nothing about findings: those are
// in the result.
func PrintReceipt(w io.Writer, r *ciapi.Receipt, resultURL string) {
	fmt.Fprintf(w, "receipt %s: %s\n", r.Id, r.Status)
	fmt.Fprintf(w, "records: %d total, %d accepted, %d rejected, %d replays, %d conflicts\n",
		r.RecordCount, r.AcceptedCount, r.RejectedCount, r.ReplayCount, r.ConflictCount)
	if r.ReasonCode != nil && *r.ReasonCode != "" {
		fmt.Fprintln(w, "reason: "+*r.ReasonCode)
	}
	if len(r.AffectedMetrics) > 0 {
		fmt.Fprintln(w, "affected metrics: "+strings.Join(r.AffectedMetrics, ", "))
	}
	if resultURL != "" {
		fmt.Fprintln(w, "result:")
		fmt.Fprintln(w, resultURL)
	}
}
