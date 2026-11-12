package dataset_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Avtobot22/craftPulse/demo-dataset/dataset"
)

func expected() dataset.ExpectedFile {
	value := json.Number("9007199254740993")
	return dataset.ExpectedFile{SchemaVersion: "1", Records: []dataset.Expected{{ID: "cycle-january", MetricID: "T08", DefinitionVersion: "1", Scope: map[string]string{"organization_id": "org-1", "team_id": "team-1"}, Filters: map[string][]string{}, Period: dataset.Window{From: "2026-01-01T00:00:00Z", To: "2026-02-01T00:00:00Z"}, Step: "month", AsOf: "2026-12-01T00:00:00Z", Unit: "seconds", Value: &value, N: 1, State: "ASSESSED", Cohort: []string{"org-1/task-a"}, Intervals: []dataset.Interval{{ObjectID: "org-1/task-a", Kind: "cycle", From: "2026-01-02T00:00:00Z", To: "2026-01-05T00:00:00Z", OriginalOffset: "+07:00"}}}}}
}

func TestCompareFindsEveryChangedValue(t *testing.T) {
	t.Parallel()
	left := expected()
	right := clone(t, left)
	changed := json.Number("9007199254740992")
	right.Records[0].Value = &changed
	right.Records[0].Intervals[0].To = "2026-01-06T00:00:00Z"
	right.Records[0].Cohort = append(right.Records[0].Cohort, "org-1/task-z")
	differences, err := dataset.Compare(&left, &right)
	if err != nil {
		t.Fatal(err)
	}
	if len(differences) != 3 {
		t.Fatalf("want 3 differences: %v", differences)
	}
	text := strings.Join(differences, "\n")
	for _, field := range []string{"/value", "/intervals", "/cohort"} {
		if !strings.Contains(text, field) {
			t.Fatalf("missing diagnostic %s: %s", field, text)
		}
	}
	if strings.Contains(text, "9007199254740992") {
		t.Fatal("diagnostics exposed payload")
	}
	differences, err = dataset.Compare(&left, &left)
	if err != nil || len(differences) != 0 {
		t.Fatalf("self comparison: %v %v", differences, err)
	}
}

func TestCompareIgnoresRecordAndCohortOrder(t *testing.T) {
	t.Parallel()
	left := expected()
	left.Records[0].Cohort = append(left.Records[0].Cohort, "org-1/task-b")
	second := clone(t, left.Records[0])
	second.ID = "other"
	left.Records = append(left.Records, second)
	right := clone(t, left)
	right.Records[0], right.Records[1] = right.Records[1], right.Records[0]
	right.Records[0].Cohort[0], right.Records[0].Cohort[1] = right.Records[0].Cohort[1], right.Records[0].Cohort[0]
	equivalent := json.Number("9007199254740993.0")
	right.Records[0].Value = &equivalent
	differences, err := dataset.Compare(&left, &right)
	if err != nil || len(differences) != 0 {
		t.Fatalf("semantic equality: %v %v", differences, err)
	}
	right.Records = right.Records[:1]
	differences, err = dataset.Compare(&left, &right)
	if err != nil || len(differences) != 1 {
		t.Fatalf("missing record: %v %v", differences, err)
	}
}

func TestDecodeAndValidateRejectMalformedExpected(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing nullable value", mutate: func(record map[string]any) { delete(record, "value") }},
		{name: "null map value", mutate: func(record map[string]any) { record["scope"] = map[string]any{"team_id": nil} }},
		{name: "null filter element", mutate: func(record map[string]any) { record["filters"] = map[string]any{"team": []any{nil}} }},
		{name: "case alias", mutate: func(record map[string]any) { record["Value"] = 123 }},
		{name: "quoted number", mutate: func(record map[string]any) { record["value"] = "123" }},
		{name: "quoted numerator", mutate: func(record map[string]any) { record["numerator"] = "123" }},
		{name: "missing n", mutate: func(record map[string]any) { delete(record, "n") }},
		{name: "unknown field", mutate: func(record map[string]any) { record["token"] = "not-for-output" }},
		{name: "null cohort", mutate: func(record map[string]any) { record["cohort"] = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(expected())
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			records, ok := raw["records"].([]any)
			if !ok {
				t.Fatal("records")
			}
			record, ok := records[0].(map[string]any)
			if !ok {
				t.Fatal("record")
			}
			test.mutate(record)
			data, err = json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var target dataset.ExpectedFile
			if err := dataset.Decode(data, &target); err == nil {
				t.Fatal("accepted invalid structure")
			}
		})
	}
	for _, data := range []string{`{"schema_version":"1","schema_version":"1","records":[]}`, `{"schema_version":"1","records":[]} {}`} {
		var target dataset.ExpectedFile
		if err := dataset.Decode([]byte(data), &target); err == nil {
			t.Fatal("accepted ambiguous JSON")
		}
	}
	for _, mutate := range []func(*dataset.Expected){
		func(r *dataset.Expected) { r.State = "CLEAN" },
		func(r *dataset.Expected) { r.AsOf = "2026-12-01T00:00:00.1Z" },
		func(r *dataset.Expected) { r.Cohort = append(r.Cohort, r.Cohort[0]) },
		func(r *dataset.Expected) { r.Intervals[0].To = r.Intervals[0].From },
	} {
		file := expected()
		mutate(&file.Records[0])
		if err := file.Validate(); err == nil {
			t.Fatal("accepted invalid expected values")
		}
	}
}
