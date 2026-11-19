package normalize

import (
	"encoding/json"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

type repositoryCandidate struct {
	key      entity.SourceKey
	snapshot bool
	version  entity.RepositoryVersion
}

type mrCandidate struct {
	key, area entity.SourceKey
	snapshot  bool
	author    string
	reviewers []string
	version   entity.MRVersion
}

type mrEventCandidate struct {
	mr            entity.SourceKey
	actor, target string
	event         entity.MREvent
}

type mrRevisionCandidate struct {
	mr       entity.SourceKey
	revision entity.MRRevision
}

func (in *input) mrKey(id string) entity.SourceKey {
	return entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectMergeRequest, NativeID: id}
}

func (in *input) repository(rec *envelope.Record, b baseFact) (*repositoryCandidate, error) {
	var p envelope.RepositoryV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode repository %s: %w", rec.NativeId, err)
	}
	return &repositoryCandidate{
		key: entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectRepository, NativeID: rec.NativeId}, snapshot: string(rec.RecordClass) == _classSnapshot,
		version: entity.RepositoryVersion{
			Record: b.ref, Revision: b.revision, ObservedAt: b.observed, SourceUpdatedAt: p.UpdatedAt,
			Path: p.Path, Name: p.Name, DefaultBranch: deref(p.DefaultBranch), Visibility: string(p.Visibility), WebURL: p.WebUrl,
		},
	}, nil
}

func (in *input) mergeRequest(rec *envelope.Record, b baseFact) (*mrCandidate, error) {
	var p envelope.MergeRequestV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode MR %s: %w", rec.NativeId, err)
	}
	c := &mrCandidate{
		key: in.mrKey(rec.NativeId), area: entity.SourceKey{InstanceID: in.instance, Kind: entity.ObjectRepository, NativeID: p.ProjectNativeId},
		snapshot: string(rec.RecordClass) == _classSnapshot, author: in.refer(p.AuthorAccountNativeId),
		version: entity.MRVersion{
			Record: b.ref, Revision: b.revision, ObservedAt: b.observed, SourceUpdatedAt: p.UpdatedAt,
			IID: p.Iid, Title: deref(p.Title), State: string(p.State), Draft: p.Draft, SourceBranch: p.SourceBranch, TargetBranch: p.TargetBranch,
			HeadSHA: p.HeadSha, BaseSHA: deref(p.BaseSha), MergeBaseSHA: deref(p.MergeBaseSha), OpenedAt: p.OpenedAt,
			ReadyAt: p.ReadyAt, MergedAt: p.MergedAt, ClosedAt: p.ClosedAt,
			ReviewerField: p.ReviewerField != nil && *p.ReviewerField,
		},
	}
	if c.version.State == "merged" && p.MergedAt == nil {
		return nil, errorf("merged MR %s has no merge time", rec.NativeId)
	}
	if c.version.ReviewerField && p.ReviewerAccountNativeIds != nil {
		for _, id := range *p.ReviewerAccountNativeIds {
			c.reviewers = append(c.reviewers, in.refer(&id))
		}
	}
	return c, nil
}

func (in *input) mrEvent(rec *envelope.Record, b baseFact, via entity.ObservedVia) (*mrEventCandidate, error) {
	// Both supported payload versions share these fields; v2 adds system_note.
	var p struct {
		envelope.MrEventV1
		SystemNote bool `json:"system_note"`
	}
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode MR event %s: %w", rec.NativeId, err)
	}
	if p.SystemNote && (p.EventKind == "comment" || p.EventKind == "review") {
		return nil, nil
	}
	if rec.EventTime == nil {
		return nil, errorf("MR event %s has no event time", rec.NativeId)
	}
	_, offset := rec.EventTime.Zone()
	reviewState := ""
	if p.ReviewState != nil {
		reviewState = string(*p.ReviewState)
	}
	return &mrEventCandidate{
		mr: in.mrKey(p.MrNativeId), actor: in.refer(p.ActorAccountNativeId), target: in.refer(p.TargetAccountNativeId),
		event: entity.MREvent{
			Record: b.ref, Revision: b.revision, EventKey: rec.NativeId, Kind: string(p.EventKind), HeadSHA: deref(p.HeadSha),
			ReviewState: reviewState, EventTime: rec.EventTime.UTC(), EventOffsetS: offset, ObservedAt: b.observed, ObservedVia: via,
		},
	}, nil
}

func (in *input) mrRevision(rec *envelope.Record, b baseFact) (*mrRevisionCandidate, error) {
	var p envelope.MrRevisionV1
	if err := json.Unmarshal(rec.Payload, &p); err != nil {
		return nil, errorf("decode MR revision %s: %w", rec.NativeId, err)
	}
	return &mrRevisionCandidate{mr: in.mrKey(p.MrNativeId), revision: entity.MRRevision{
		Record: b.ref, Revision: b.revision, RevisionKey: rec.NativeId,
		HeadSHA: p.HeadSha, BaseSHA: deref(p.BaseSha), StartSHA: deref(p.StartSha), CreatedAt: p.CreatedAt, ObservedAt: b.observed,
	}}, nil
}

func olderTimes(updated *time.Time, observed time.Time, previous *time.Time, received time.Time, snapshot bool) bool {
	if !snapshot && updated != nil && previous != nil && !updated.Equal(*previous) {
		return updated.Before(*previous)
	}
	return observed.Before(received)
}
