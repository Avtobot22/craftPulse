package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_maxSchemaVersion = 16
	_maxInstanceKey   = 512
)

// _importAreas are the kinds of area a schema of the import describes; the project of the form names it.
var _importAreas = map[string]envelope.AreaRefKind{
	"tasks": envelope.AreaRefKindTrackerProject, "task_events": envelope.AreaRefKindTrackerProject,
	"relations": envelope.AreaRefKindTrackerProject, "merge_requests": envelope.AreaRefKindRepository,
	"ci_runs": envelope.AreaRefKindCiProject, "documents": envelope.AreaRefKindWikiSpace,
}

// Import packs a file of the universal import into the package CI sends and passes it to the intake of
// the import channel (TZ 10.5.2). JSON is an array of records of the envelope, CSV is a snapshot of
// current tasks without history (FR-016 A1). The manifest is built from the form; the read of a snapshot
// is named by the hash of the file, so the same file sent again repeats the same read.
func (u *UseCase) Import(ctx context.Context, s entity.Subject, f entity.ImportFile) (entity.Receipt, error) {
	if err := validateImport(f); err != nil {
		return entity.Receipt{}, err
	}
	area := _importAreas[f.SchemaID]
	m := &envelope.Manifest{
		SchemaId: envelope.ManifestSchemaId(f.SchemaID), SchemaVersion: f.SchemaVersion,
		Source:       envelope.Source{Provider: envelope.SourceProvider(f.Provider), InstanceKey: f.InstanceKey},
		Area:         envelope.AreaRef{Kind: area, NativeId: f.ProjectID.String()},
		Completeness: envelope.ManifestCompletenessUnknown,
		Selection:    envelope.Selection{End: true, Truncated: []envelope.TruncatedHistory{}},
		Snapshot:     f.Snapshot || f.Format == entity.ImportCSV,
		Producer:     envelope.Producer{Name: "craftpulse-import", Version: "1"},
		CreatedAt:    u.d.Clock.Now(),
	}
	if m.Snapshot {
		sum := sha256.Sum256(f.Content)
		read := "import:" + hex.EncodeToString(sum[:])
		m.ReadId = &read
	}
	var pkg bytes.Buffer
	w, err := envelope.NewWriter(&pkg, m)
	if err != nil {
		return entity.Receipt{}, err
	}
	if f.Format == entity.ImportCSV {
		err = w.AddTasksCSV(f.Content)
	} else {
		err = addJSONRecords(w, f.Content)
	}
	if err != nil {
		return entity.Receipt{}, err
	}
	if err := w.Close(); err != nil {
		return entity.Receipt{}, err
	}
	return u.SubmitImport(ctx, s, f.ProjectID, &pkg)
}

// addJSONRecords writes every element of the array as one line of records.jsonl; the intake checks them.
func addJSONRecords(w *envelope.Writer, content []byte) error {
	var records []json.RawMessage
	if err := json.Unmarshal(content, &records); err != nil {
		return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/file", Code: entity.FieldInvalidFormat, Detail: "a JSON array of records"}}}
	}
	var lines bytes.Buffer
	for _, r := range records {
		if err := json.Compact(&lines, r); err != nil {
			return &entity.ValidationError{Fields: []entity.FieldError{{Field: "/file", Code: entity.FieldInvalidFormat}}}
		}
		lines.WriteByte('\n')
	}
	return w.AddRecordsFrom(&lines, int64(lines.Len()))
}

var _providers = []entity.Provider{
	entity.ProviderJiraCloud, entity.ProviderGitLab, entity.ProviderGitHub, entity.ProviderJenkins, entity.ProviderSonarQube,
	entity.ProviderConfluenceCloud, entity.ProviderGit, entity.ProviderUniversal, entity.ProviderClient,
}

func validateImport(f entity.ImportFile) error {
	v := &entity.ValidationError{}
	if !slices.Contains(_providers, f.Provider) {
		v.Add("/source_provider", entity.FieldNotAllowedValue, "")
	}
	if _, ok := _importAreas[f.SchemaID]; !ok {
		v.Add("/schema_id", entity.FieldNotAllowedValue, "reports of findings, tests and coverage come with the CI step")
	}
	if f.SchemaVersion == "" || len(f.SchemaVersion) > _maxSchemaVersion {
		v.Add("/schema_version", entity.FieldOutOfRange, "1 to 16 characters")
	}
	if f.InstanceKey == "" || len(f.InstanceKey) > _maxInstanceKey {
		v.Add("/source_instance_key", entity.FieldOutOfRange, "1 to 512 characters")
	}
	if f.Format == entity.ImportCSV && f.SchemaID != "tasks" {
		v.Add("/file", entity.FieldNotAllowedValue, "CSV is a snapshot of tasks")
	}
	if len(v.Fields) > 0 {
		return v
	}
	return nil
}
