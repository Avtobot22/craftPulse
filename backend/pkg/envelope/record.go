package envelope

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

const (
	classEvent    = "event"
	classSnapshot = "snapshot"
	classFile     = "file"
)

func (c *checker) checkRecord(no int, line []byte) {
	result := RecordResult{No: no}
	value, err := decode(line)
	record, ok := value.(map[string]any)
	if err != nil || !ok {
		result.Rejection = &Rejection{ReasonInvalidRecord, "line is not a JSON object"}
		c.result.Records = append(c.result.Records, result)
		return
	}
	c.addRecord(result, record)
}

// addRecord validates a decoded record and keeps its result; file references are resolved by checkFiles.
func (c *checker) addRecord(result RecordResult, record map[string]any) {
	result.Kind, _ = record["record_kind"].(string)
	result.NativeNS, _ = record["native_ns"].(string)
	result.NativeID, _ = record["native_id"].(string)
	rule, rejection := c.recordRule(record)
	if rejection == nil {
		result.Hash, rejection = recordHash(record, rule)
	}
	result.Rejection = rejection
	c.result.Records = append(c.result.Records, result)
	if rejection != nil {
		return
	}
	index := len(c.result.Records) - 1
	if file, ok := record["file"].(map[string]any); ok {
		size, _ := file["size_bytes"].(json.Number)
		n, _ := size.Int64()
		c.fileRefs = append(c.fileRefs, pendingFile{result: index, path: text(file, "path"), sha256: text(file, "sha256"), size: n, verify: true})
	}
	if payload, ok := record["payload"].(map[string]any); ok {
		if content, ok := payload["content_path"].(string); ok {
			c.fileRefs = append(c.fileRefs, pendingFile{result: index, path: content})
		}
	}
}

// recordRule checks the envelope of a record, its kind and class against the package, then the payload.
func (c *checker) recordRule(record map[string]any) (recordRule, *Rejection) {
	if id, _ := record["native_id"].(string); id == "" {
		return recordRule{}, &Rejection{ReasonMissingNativeID, "native_id is required"}
	}
	class, _ := record["record_class"].(string)
	if class == classEvent && record["event_time"] == nil {
		return recordRule{}, &Rejection{ReasonMissingEventTime, "event_time is required for an event"}
	}
	if bad := _rules.Record.validate(record, ""); bad != nil {
		return recordRule{}, &Rejection{ReasonInvalidRecord, bad.Error()}
	}
	kind := text(record, "record_kind")
	if !slices.Contains(c.rule.Kinds, kind) {
		return recordRule{}, &Rejection{ReasonKindNotAllowed, kind + " is not part of schema_id " + text(c.manifest, "schema_id")}
	}
	rule, ok := _rules.record(text(record, "payload_schema"))
	if !ok || rule.Kind != kind {
		return recordRule{}, &Rejection{ReasonUnsupportedPayloadSchema, "payload_schema is not a supported schema of " + kind}
	}
	if !slices.Contains(rule.Classes, class) {
		return recordRule{}, &Rejection{ReasonClassNotAllowed, class + " is not a class of " + kind}
	}
	if c.snapshot() && class != classSnapshot {
		return recordRule{}, &Rejection{ReasonNotSnapshot, "a snapshot package accepts only snapshot records"}
	}
	if class == classSnapshot && c.manifest["read_id"] == nil {
		return recordRule{}, &Rejection{ReasonMissingReadID, "snapshot record needs read_id in the manifest"}
	}
	if (class == classFile) != (record["file"] != nil) {
		return recordRule{}, &Rejection{ReasonInvalidRecord, "file: set only for file records"}
	}
	if bad := rule.Schema.validate(record["payload"], "payload"); bad != nil {
		return recordRule{}, &Rejection{ReasonSchemaViolation, bad.Error()}
	}
	payload, _ := record["payload"].(map[string]any) // validated as an object above
	if problem := semantic(kind, payload); problem != "" {
		return recordRule{}, &Rejection{ReasonSchemaViolation, problem}
	}
	return rule, nil
}

// semantic checks the conditions between fields that the schema cannot express.
func semantic(kind string, payload map[string]any) string {
	switch kind {
	case "task_event":
		if payload["event_kind"] == "field_change" && payload["field_native_id"] == nil {
			return "payload.field_native_id: required for field_change"
		}
	case "report_file":
		if payload["sha"] == nil && payload["artifact_digest"] == nil {
			return "payload.sha: sha or artifact_digest is required"
		}
	case "document":
		if payload["path"] == nil && payload["page_native_id"] == nil {
			return "payload.path: path or page_native_id is required"
		}
	}
	return ""
}

func recordHash(record map[string]any, rule recordRule) (string, *Rejection) {
	var eventTime *string
	if t, ok := record["event_time"].(string); ok {
		eventTime = &t
	}
	var fileSHA string
	if file, ok := record["file"].(map[string]any); ok {
		fileSHA, _ = file["sha256"].(string)
	}
	hash, err := hashContent(eventTime, rule.Schema, record["payload"], fileSHA)
	if err != nil {
		return "", &Rejection{ReasonInvalidRecord, "payload cannot be canonicalized"}
	}
	return hash, nil
}

// readTasksCSV turns every row of a current-state task CSV into a snapshot task record (FR-016 A1).
func (c *checker) readTasksCSV(r io.Reader) *Rejection {
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		return &Rejection{ReasonInvalidCSV, "header row is missing"}
	}
	seen := map[string]bool{}
	for _, column := range header {
		if _rules.TaskCSVRow.property(column) == nil || seen[column] {
			return &Rejection{ReasonInvalidCSV, fmt.Sprintf("column %q is unknown or repeated", column)}
		}
		seen[column] = true
	}
	if !seen["native_id"] {
		return &Rejection{ReasonInvalidCSV, "column native_id is required"}
	}
	for no := 1; ; no++ {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		var parseErr *csv.ParseError
		if errors.As(err, &parseErr) && errors.Is(parseErr.Err, csv.ErrFieldCount) {
			c.result.Records = append(c.result.Records, RecordResult{No: no, Kind: "task", Rejection: &Rejection{ReasonInvalidRecord, "row has a different number of columns"}})
			continue
		}
		if err != nil {
			return &Rejection{ReasonInvalidCSV, fmt.Sprintf("row %d is not valid CSV", no)}
		}
		c.addCSVRow(no, header, row)
	}
}

func (c *checker) addCSVRow(no int, header, row []string) {
	cells := map[string]any{}
	for i, column := range header {
		if row[i] != "" {
			cells[column] = row[i]
		}
	}
	if cells["native_id"] == nil {
		c.result.Records = append(c.result.Records, RecordResult{No: no, Kind: "task", Rejection: &Rejection{ReasonMissingNativeID, "native_id is required"}})
		return
	}
	if bad := _rules.TaskCSVRow.validate(cells, ""); bad != nil {
		id, _ := cells["native_id"].(string)
		c.result.Records = append(c.result.Records, RecordResult{No: no, Kind: "task", NativeID: id, Rejection: &Rejection{ReasonSchemaViolation, bad.Error()}})
		return
	}
	id := cells["native_id"]
	delete(cells, "native_id")
	c.addRecord(RecordResult{No: no}, map[string]any{
		"record_class": classSnapshot, "record_kind": "task", "payload_schema": "task/1",
		"native_ns": "", "native_id": id, "source_revision": nil, "event_time": nil,
		"observed_time": nil, "file": nil, "payload": cells,
	})
}
