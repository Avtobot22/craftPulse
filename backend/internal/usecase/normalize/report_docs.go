package normalize

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _legacyDocsProfile = "git-markdown-openapi"

func parseDocsInventory(r io.Reader, in parseInput) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	var raw envelope.DocsInventory
	if err := dec.Decode(&raw); err != nil {
		return nil, invalidReport("docs inventory: %v", err)
	}
	if err := end(dec); err != nil {
		return nil, err
	}
	if raw.FormatVersion != "1" {
		return &parsedReport{formatVersion: raw.FormatVersion}, nil
	}
	if in.container == "" || in.sha == "" || raw.PathPrefix != "" && !documentPath(raw.PathPrefix, _maxPath) {
		return nil, invalidReport("docs inventory: invalid SHA or root path")
	}
	slices.Sort(raw.Checks)
	docs := &entity.DocsReportFacts{PathPrefix: raw.PathPrefix, OwnerRule: docsText(raw.OwnerRule, 128), Checks: slices.Compact(raw.Checks)}
	if raw.RootID != nil {
		id := entity.ID(*raw.RootID)
		if id.IsZero() {
			return nil, invalidReport("docs inventory: empty root ID")
		}
		docs.RootID = &id
	}
	if raw.Links != nil {
		l := raw.Links
		if in.kind != entity.AnalysisDocs || l.Checked < 0 || l.Broken < 0 || l.External < 0 || l.Broken > l.Checked {
			return nil, invalidReport("docs inventory: invalid link counts")
		}
		docs.Links = &entity.DocsLinkCounts{Checked: l.Checked, Broken: l.Broken, External: l.External}
	}
	for _, kind := range raw.Checks {
		if kind != entity.AnalysisDocs && kind != entity.AnalysisOpenAPI {
			return nil, invalidReport("docs inventory: invalid check")
		}
	}
	if len(raw.Checks) != 0 && !slices.Contains(raw.Checks, in.kind) {
		return nil, invalidReport("docs inventory: check absent from profile scope")
	}
	seen := make(map[string]bool, len(raw.Documents))
	for i := range raw.Documents {
		doc, err := docsEntry(&raw.Documents[i], in, raw.PathPrefix)
		if err != nil {
			return nil, err
		}
		if seen[doc.Path] {
			return nil, invalidReport("docs inventory: duplicate path")
		}
		seen[doc.Path] = true
		docs.Documents = append(docs.Documents, doc)
	}
	return &parsedReport{formatVersion: raw.FormatVersion, docs: docs}, nil
}

func docsEntry(entry *envelope.DocsEntry, in parseInput, prefix string) (entity.DocsReportDocument, error) {
	doc := &entry.Document
	p := deref(doc.Path)
	if !documentPath(p, _maxPath) || (prefix != "" && p != prefix && !strings.HasPrefix(p, prefix+"/")) ||
		doc.VersionRef != in.sha || doc.ContainerNativeId != in.container || doc.AclState != envelope.DocumentV1AclStateKnown ||
		(doc.CommitSha != nil && *doc.CommitSha != in.sha) {
		return entity.DocsReportDocument{}, invalidReport("docs inventory: invalid document scope or version")
	}
	switch doc.DocFormat {
	case envelope.DocumentV1DocFormatMarkdown:
		if in.kind != entity.AnalysisDocs || entry.SpecValid != nil {
			return entity.DocsReportDocument{}, invalidReport("docs inventory: Markdown in wrong check")
		}
	case envelope.DocumentV1DocFormatOpenapi:
		if in.kind == entity.AnalysisOpenAPI && entry.SpecValid == nil || in.kind == entity.AnalysisDocs && entry.SpecValid != nil {
			return entity.DocsReportDocument{}, invalidReport("docs inventory: unexpected OpenAPI validity")
		}
	default:
		return entity.DocsReportDocument{}, invalidReport("docs inventory: unsupported document format")
	}
	hash, err := hexHash(doc.ContentSha256)
	if err != nil || len(hash) != 32 {
		return entity.DocsReportDocument{}, invalidReport("docs inventory: invalid document hash")
	}
	return entity.DocsReportDocument{
		DocumentVersion: entity.DocumentVersion{
			Path: p, VersionRef: doc.VersionRef, Format: string(doc.DocFormat), Title: docsText(deref(doc.Title), _maxDescription),
			ContentHash: hash, Owner: docsText(strings.TrimSpace(deref(entry.Owner)), _maxDescription), ACLState: string(doc.AclState),
		},
		SpecValid: entry.SpecValid, EventTime: doc.UpdatedAt, EventOffsetS: sourceTimeOffset(doc.UpdatedAt),
	}, nil
}

func docsText(s string, limit int) string {
	return bounded(strings.ReplaceAll(s, "\x00", ""), limit)
}

func documentPath(p string, limit int) bool {
	return p != "" && p != "." && len(p) <= limit && repoPath(p) == p && path.Clean(p) == p && !strings.ContainsRune(p, 0)
}

func (c *reportCandidate) docsProvenance(parsed *parsedReport) {
	if parsed.docs == nil {
		return
	}
	if c.report.ScannedCount != nil && *c.report.ScannedCount != len(parsed.docs.Documents) {
		c.rejected = entity.RecordReportInvalid
		return
	}
	c.docs = parsed.docs
	if c.tool == nil {
		return
	}
	if c.report.Kind == entity.AnalysisDocs && c.docs.OwnerRule == "" && c.report.ProfileID == _legacyDocsProfile {
		c.docs.OwnerRule = "owner"
	}
	for _, r := range c.tool.Reports {
		if r.ReportFormat == envelope.ScanReportReportFormatSarif && string(r.ReportKind) == c.report.Kind {
			c.docs.SARIFHash, _ = hexHash(&r.File.Sha256) // validated scan manifest
			break
		}
	}
}

func (in *input) documentKey(p string) entity.SourceKey {
	// Match the Git adapter's identity for regular paths. Hash long paths to fit the native ID.
	id := p
	if len(id) > 1024 {
		id = "/sha256:" + hex.EncodeToString(fingerprint("git-document", p))
	}
	return entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectDocument, NativeNS: in.area.NativeID, NativeID: id}
}
