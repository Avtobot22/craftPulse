package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type findings struct{ uc usecase.Findings }

func (h *findings) GetFinding(ctx context.Context, req openapi.GetFindingRequestObject) (openapi.GetFindingResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	f, err := h.uc.GetFinding(ctx, s, scope, entity.ID(req.FindingId))
	if err != nil {
		return nil, err
	}
	body, err := findingBody(&f)
	if err != nil {
		return nil, err
	}
	return openapi.GetFinding200JSONResponse(body), nil
}

func findingBody(f *entity.FindingDetail) (openapi.FindingDetail, error) {
	report := &f.Report
	summary, safe, err := findingSummary(f)
	if err != nil {
		return openapi.FindingDetail{}, err
	}
	out := openapi.FindingDetail{
		Id: summary.Id, FindingKind: summary.FindingKind, RuleId: summary.RuleId, State: summary.State,
		FirstSeenAt: summary.FirstSeenAt, ComponentId: summary.ComponentId, Confidence: summary.Confidence,
		Location: summary.Location, Severity: summary.Severity, Package: summary.Package,
		Actions:  make([]openapi.FindingDetailActions, 0, len(f.Actions)),
		Evidence: map[string]string{}, Redactions: openapi.Redactions{},
	}
	out.Exceptions = emptyOf(out.Exceptions, len(f.Exceptions))
	out.ExternalTriage = emptyOf(out.ExternalTriage, 0)
	for _, action := range f.Actions {
		out.Actions = append(out.Actions, openapi.FindingDetailActions(action))
	}

	out.Report.Id, out.Report.Sha = uuid.UUID(report.ID), optional(report.SHA)
	out.Report.Completeness = openapi.FindingDetailReportCompleteness(report.Completeness)
	out.Report.Producer, out.Report.ProducerVersion = report.Producer, report.ProducerVersion
	out.Report.ProfileId = optional(report.ProfileID)
	out.Report.Mode = new(openapi.FindingDetailReportMode(report.Mode))
	out.Report.RulesVersion = optional(hex.EncodeToString(report.RulesHash))
	out.Report.DbVersion = optional(report.DBVersion)

	for i := range f.Exceptions {
		v := &f.Exceptions[i]
		item := elemOf(out.Exceptions)
		item.ExceptionId, item.ExpiresAt = uuid.UUID(v.DecisionID), v.ExpiresAt
		item.State = openapi.FindingDetailExceptionsStateActive
		if v.Status == entity.ExceptionExpired {
			item.State = openapi.FindingDetailExceptionsStateExpired
		}
		out.Exceptions = append(out.Exceptions, *item)
	}
	if report.Mode == "import" {
		for _, state := range safe.ExternalSuppressions {
			switch state {
			case "accepted", "rejected", "underReview":
				item := elemOf(out.ExternalTriage)
				item.Authority, item.State = report.Producer, state
				out.ExternalTriage = append(out.ExternalTriage, *item)
			}
		}
		if len(out.ExternalTriage) == 0 && safe.Suppressed && f.Kind != entity.FindingSecret {
			item := elemOf(out.ExternalTriage)
			item.Authority, item.State = report.Producer, "suppressed"
			out.ExternalTriage = append(out.ExternalTriage, *item)
		}
	}
	if f.Kind == entity.FindingSecret {
		return out, nil
	}
	for key, value := range map[string]string{
		"rule_name": safe.RuleName, "rule_description": safe.RuleDescription,
		"target_path": safe.Link.TargetPath, "anchor": safe.Link.Anchor,
	} {
		if value != "" {
			runes := []rune(value)
			out.Evidence[key] = string(runes[:min(len(runes), 4096)])
		}
	}
	return out, nil
}

type findingEvidence struct {
	ExternalSuppressions []string                   `json:"external_suppressions"`
	RuleName             string                     `json:"rule_name"`
	RuleDescription      string                     `json:"rule_description"`
	Package              *entity.DependencyEvidence `json:"package"`
	Suppressed           bool                       `json:"suppressed"`
	Link                 envelope.DocsLinkTarget    `json:"link"`
}

func findingSummary(f *entity.FindingDetail) (openapi.Finding, findingEvidence, error) {
	o := &f.Observation
	out := openapi.Finding{
		Id: uuid.UUID(f.ID), FindingKind: openapi.FindingKind(f.Kind), RuleId: o.RuleID,
		State: openapi.FindingState(f.State), FirstSeenAt: f.FirstSeenAt, ComponentId: uuidPtr(f.ComponentID),
	}
	if !strings.EqualFold(strings.TrimSpace(o.NativeConfidence), "unknown") {
		out.Confidence = optional(o.NativeConfidence)
	}
	if o.Path != "" {
		out.Location = &openapi.CodeLocation{Path: o.Path, LineStart: o.LineStart, LineEnd: o.LineEnd}
	}
	out.Severity.Native = optional(o.NativeSeverity)
	if o.MappedSeverity != "" {
		out.Severity.Mapped = new(openapi.SeverityMapped(o.MappedSeverity))
		if o.SeverityMappingVersion > 0 {
			out.Severity.MappingVersion = &o.SeverityMappingVersion
		}
	}
	var safe findingEvidence
	if len(o.Evidence) > 0 {
		if err := json.Unmarshal(o.Evidence, &safe); err != nil {
			return out, safe, fmt.Errorf("decode finding evidence: %w", err)
		}
	}
	if f.Kind != entity.FindingSecret && o.PackagePURL != "" {
		out.Package = &struct {
			Purl    string  `json:"purl"`
			Version *string `json:"version"`
		}{}
		out.Package.Purl = o.PackagePURL
		if safe.Package != nil {
			out.Package.Version = optional(safe.Package.Version)
		}
	}
	return out, safe, nil
}
