package normalize

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) ReportedRecords(ctx context.Context, orgID, receiptID entity.ID, nos []int) ([]int, error) {
	list := make([]int32, len(nos))
	for i, no := range nos {
		n, err := recordNo(no)
		if err != nil {
			return nil, err
		}
		list[i] = n
	}
	rows, err := r.q(ctx).ReportedRecords(ctx, ReportedRecordsParams{OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(receiptID), RecordNos: list})
	if err != nil {
		return nil, fmt.Errorf("normalize: reported records: %w", err)
	}
	out := make([]int, len(rows))
	for i, no := range rows {
		out[i] = int(no)
	}
	return out, nil
}

// InsertReport writes a report with its rows in the caller's transaction.
func (r *Repo) InsertReport(ctx context.Context, orgID entity.ID, rep *entity.ScanReport, rows entity.ReportRows) error {
	no, err := recordNo(rep.Record.RecordNo)
	if err != nil {
		return err
	}
	skipped, err := json.Marshal(rep.SkippedTargets)
	if err != nil {
		return fmt.Errorf("normalize: encode skipped targets: %w", err)
	}
	if rep.SkippedTargets == nil {
		skipped = []byte("[]")
	}
	p := InsertReportParams{
		OrganizationID: uuid.UUID(orgID), ReceiptID: uuid.UUID(rep.Record.ReceiptID), RecordNo: no, KnownSeq: rep.KnownSeq,
		AreaID: uuid.UUID(rep.AreaID), ComponentID: nullID(rep.ComponentID), ReportKind: rep.Kind, Mode: rep.Mode,
		Producer: text(rep.Producer), ProducerVersion: text(rep.ProducerVersion), ReportFormat: rep.Format,
		SchemaVersion: rep.SchemaVersion, ParserVersion: rep.ParserVersion, ProfileID: text(rep.ProfileID),
		RulesHash: rep.RulesHash, ConfigHash: rep.ConfigHash, DbVersion: text(rep.DBVersion), DbPublishedAt: tstz(rep.DBPublishedAt),
		ScopeHash: rep.ScopeHash, Sha: text(rep.SHA), ArtifactDigest: text(rep.ArtifactDigest), RunID: nullID(rep.RunID),
		AttemptID: nullID(rep.AttemptID), StartedAt: tstz(rep.StartedAt), FinishedAt: tstz(rep.FinishedAt), ExitClass: rep.ExitClass,
		Completeness: rep.Completeness, SkippedTargets: skipped,
	}
	if p.ScannedCount, err = int4(rep.ScannedCount); err != nil {
		return err
	}
	if p.SkippedCount, err = int4(rep.SkippedCount); err != nil {
		return err
	}
	if h := rep.History; h != nil {
		p.HistoryFromSha, p.HistoryToSha, p.HistoryTruncated = text(h.FromSHA), text(h.ToSHA), pgtype.Bool{Bool: h.Truncated, Valid: true}
	}
	id, err := r.q(ctx).InsertReport(ctx, p)
	if err != nil {
		return fmt.Errorf("normalize: insert report: %w", err)
	}
	if err := r.insertFindings(ctx, orgID, id, rep.KnownSeq, rows.Findings); err != nil {
		return err
	}
	if err := r.insertTests(ctx, orgID, id, rep, rows.Tests); err != nil {
		return err
	}
	return r.insertCoverage(ctx, orgID, id, rep.KnownSeq, rows.Coverage)
}

func (r *Repo) insertFindings(ctx context.Context, orgID entity.ID, id uuid.UUID, seq int64, findings []entity.FindingOccurrence) error {
	if len(findings) == 0 {
		return nil
	}
	var err error
	f := InsertFindingsParams{
		OrganizationID: uuid.UUID(orgID), ReportID: id, KnownSeq: seq,
		NativeIds: make([]string, len(findings)), RuleIds: make([]string, len(findings)), FindingKinds: make([]string, len(findings)),
		Paths: make([]string, len(findings)), LineStarts: make([]int32, len(findings)), LineEnds: make([]int32, len(findings)),
		PackagePurls: make([]string, len(findings)), NativeSeverities: make([]string, len(findings)),
		NativeConfidences: make([]string, len(findings)), Fingerprints: make([][]byte, len(findings)), Evidences: make([]string, len(findings)),
	}
	for i := range findings {
		o := &findings[i]
		if f.LineStarts[i], err = line(o.LineStart); err != nil {
			return err
		}
		if f.LineEnds[i], err = line(o.LineEnd); err != nil {
			return err
		}
		f.NativeIds[i], f.RuleIds[i], f.FindingKinds[i], f.Paths[i] = o.NativeID, o.RuleID, o.Kind, o.Path
		f.PackagePurls[i], f.NativeSeverities[i], f.NativeConfidences[i] = o.PackagePURL, o.NativeSeverity, o.NativeConfidence
		f.Fingerprints[i], f.Evidences[i] = o.Fingerprint, string(o.Evidence)
	}
	if err := r.q(ctx).InsertFindings(ctx, f); err != nil {
		return fmt.Errorf("normalize: insert findings: %w", err)
	}
	return nil
}

// insertTests keeps the cases of the area and writes the executions of a report; every execution of a report has its
// attempt and run start.
func (r *Repo) insertTests(ctx context.Context, orgID entity.ID, id uuid.UUID, rep *entity.ScanReport, tests []entity.TestExecution) error {
	if len(tests) == 0 {
		return nil
	}
	org, area := uuid.UUID(orgID), uuid.UUID(rep.AreaID)
	cases := map[string]string{}
	for i := range tests {
		cases[string(tests[i].CaseKeyHash)] = tests[i].CaseKey
	}
	keys, hashes := make([]string, 0, len(cases)), make([][]byte, 0, len(cases))
	for hash, key := range cases {
		keys, hashes = append(keys, key), append(hashes, []byte(hash))
	}
	if err := r.q(ctx).EnsureTestCases(ctx, EnsureTestCasesParams{OrganizationID: org, AreaID: area, CaseKeys: keys, CaseKeyHashes: hashes}); err != nil {
		return fmt.Errorf("normalize: ensure test cases: %w", err)
	}
	found, err := r.q(ctx).TestCaseIDs(ctx, TestCaseIDsParams{OrganizationID: org, AreaID: area, CaseKeyHashes: hashes})
	if err != nil {
		return fmt.Errorf("normalize: test case ids: %w", err)
	}
	ids := make(map[string]uuid.UUID, len(found))
	for _, row := range found {
		ids[string(row.CaseKeyHash)] = row.ID
	}
	p := InsertTestExecutionsParams{
		OrganizationID: org, ReportID: id, AttemptID: nullID(tests[0].AttemptID), RunStartedAt: tests[0].RunStartedAt, KnownSeq: rep.KnownSeq,
		TestCaseIds: make([]uuid.UUID, len(tests)), ConfigFingerprints: make([][]byte, len(tests)), Ordinals: make([]int16, len(tests)),
		Outcomes: make([]string, len(tests)), DurationsMs: make([]int64, len(tests)),
	}
	for i := range tests {
		t := &tests[i]
		if t.Ordinal < 1 || t.Ordinal > math.MaxInt16 {
			return fmt.Errorf("normalize: ordinal %d of a test does not fit its column", t.Ordinal)
		}
		p.TestCaseIds[i], p.ConfigFingerprints[i], p.Ordinals[i], p.Outcomes[i] = ids[string(t.CaseKeyHash)], t.ConfigFingerprint, int16(t.Ordinal), t.Outcome
		if t.DurationMS != nil && *t.DurationMS < math.MaxInt64 {
			p.DurationsMs[i] = *t.DurationMS + 1
		}
	}
	if err := r.q(ctx).InsertTestExecutions(ctx, p); err != nil {
		return fmt.Errorf("normalize: insert test executions: %w", err)
	}
	return nil
}

func (r *Repo) insertCoverage(ctx context.Context, orgID entity.ID, id uuid.UUID, seq int64, facts []entity.CoverageFact) error {
	if len(facts) == 0 {
		return nil
	}
	p := InsertCoverageFactsParams{
		OrganizationID: uuid.UUID(orgID), ReportID: id, KnownSeq: seq,
		Paths: make([]string, len(facts)), PathHashes: make([][]byte, len(facts)), Kinds: make([]string, len(facts)),
		Covered: make([]int32, len(facts)), Executable: make([]int32, len(facts)), LineMapKeys: make([]string, len(facts)),
	}
	var err error
	for i := range facts {
		f := &facts[i]
		if p.Covered[i], err = lineCount(f.Covered); err != nil {
			return err
		}
		if p.Executable[i], err = lineCount(f.Executable); err != nil {
			return err
		}
		if f.Covered > f.Executable {
			return fmt.Errorf("normalize: %s covers %d of %d", f.Path, f.Covered, f.Executable)
		}
		sum := sha256.Sum256([]byte(f.Path))
		p.Paths[i], p.PathHashes[i], p.Kinds[i], p.LineMapKeys[i] = f.Path, sum[:], f.Kind, f.LineMapKey
	}
	if err := r.q(ctx).InsertCoverageFacts(ctx, p); err != nil {
		return fmt.Errorf("normalize: insert coverage facts: %w", err)
	}
	return nil
}

func int4(n *int) (pgtype.Int4, error) {
	if n == nil {
		return pgtype.Int4{}, nil
	}
	if *n < 0 || *n > math.MaxInt32 {
		return pgtype.Int4{}, fmt.Errorf("normalize: count %d does not fit its column", *n)
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}, nil
}

func lineCount(n int) (int32, error) {
	if n < 0 || n > math.MaxInt32 {
		return 0, fmt.Errorf("normalize: coverage count %d does not fit its column", n)
	}
	return int32(n), nil
}

// line is a line of a finding, zero when it is unknown.
func line(n *int) (int32, error) {
	if n == nil {
		return 0, nil
	}
	if *n < 1 || *n > math.MaxInt32 {
		return 0, fmt.Errorf("normalize: line %d does not fit its column", *n)
	}
	return int32(*n), nil
}
