package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
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
	o, report := &f.Observation, &f.Report
	out := openapi.FindingDetail{
		Id: uuid.UUID(f.ID), FindingKind: openapi.FindingKind(f.Kind), RuleId: o.RuleID,
		State: openapi.FindingState(f.State), FirstSeenAt: f.FirstSeenAt, ComponentId: uuidPtr(f.ComponentID),
		Confidence: optional(o.NativeConfidence), Actions: []openapi.FindingDetailActions{},
		Evidence: map[string]string{}, Redactions: openapi.Redactions{},
	}
	out.Exceptions = emptyOf(out.Exceptions, len(f.Exceptions))
	out.ExternalTriage = emptyOf(out.ExternalTriage, 0)
	if o.Path != "" {
		out.Location = &openapi.CodeLocation{Path: o.Path, LineStart: o.LineStart, LineEnd: o.LineEnd}
	}
	out.Severity.Native = optional(o.NativeSeverity)
	if o.MappedSeverity != "" {
		out.Severity.Mapped = new(openapi.SeverityMapped(o.MappedSeverity))
		out.Severity.MappingVersion = &o.SeverityMappingVersion
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
		out.Exceptions = append(out.Exceptions, *item)
	}
	err := setFindingEvidence(&out, o.Evidence, o.PackagePURL, f.Kind)
	return out, err
}

func setFindingEvidence(out *openapi.FindingDetail, evidence json.RawMessage, purl, kind string) error {
	if kind == entity.FindingSecret {
		return nil
	}
	var safe struct {
		RuleName        string                     `json:"rule_name"`
		RuleDescription string                     `json:"rule_description"`
		Package         *entity.DependencyEvidence `json:"package"`
	}
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &safe); err != nil {
			return fmt.Errorf("decode finding evidence: %w", err)
		}
	}
	for key, value := range map[string]string{"rule_name": safe.RuleName, "rule_description": safe.RuleDescription} {
		if value != "" {
			runes := []rune(value)
			out.Evidence[key] = string(runes[:min(len(runes), 4096)])
		}
	}
	if purl != "" {
		out.Package = &struct {
			Purl    string  `json:"purl"`
			Version *string `json:"version"`
		}{Purl: purl}
		if safe.Package != nil {
			out.Package.Version = optional(safe.Package.Version)
		}
	}
	return nil
}
