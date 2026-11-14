package dataset

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"sort"
)

type ExpectedFile struct {
	SchemaVersion string     `json:"schema_version"`
	Records       []Expected `json:"records"`
}

type Expected struct {
	ID                string              `json:"id"`
	MetricID          string              `json:"metric_id"`
	DefinitionVersion string              `json:"definition_version"`
	Scope             map[string]string   `json:"scope"`
	Filters           map[string][]string `json:"filters"`
	Period            Window              `json:"period"`
	Step              string              `json:"step"`
	AsOf              string              `json:"as_of"`
	Unit              string              `json:"unit"`
	Value             *json.Number        `json:"value"`
	Numerator         *json.Number        `json:"numerator"`
	Denominator       *json.Number        `json:"denominator"`
	N                 int64               `json:"n"`
	State             string              `json:"state"`
	Cohort            []string            `json:"cohort"`
	Intervals         []Interval          `json:"intervals"`
	// Flags mark a value without changing it: low_sample for n < 20 (BR-006), immature for an observation
	// window that has not ended, mapping_missing without a confirmed mapping.
	Flags []string `json:"flags"`
	// Conditions replace an exact value: a BOUNDED value known only by its bounds, or a value the stand decides
	// at load time, such as the watermark of T40. A record with conditions has a null value.
	Conditions []Condition `json:"conditions"`
}

// Condition bounds the value of the actual record: op is ge or le.
type Condition struct {
	Op    string      `json:"op"`
	Value json.Number `json:"value"`
}

type Interval struct {
	ObjectID       string `json:"object_id"`
	Kind           string `json:"kind"`
	From           string `json:"from"`
	To             string `json:"to"`
	OriginalOffset string `json:"original_offset"`
}

func (f *ExpectedFile) Validate() error {
	if f.SchemaVersion != "1" || f.Records == nil {
		return fmt.Errorf("expected file requires schema_version 1 and records array")
	}
	seen := make(map[string]bool)
	for i := range f.Records {
		record := &f.Records[i]
		if record.ID == "" || seen[record.ID] {
			return fmt.Errorf("record %d: missing or duplicate id", i)
		}
		seen[record.ID] = true
		if err := record.validate(); err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
	}
	return nil
}

func (r *Expected) validate() error {
	if r.MetricID == "" || r.DefinitionVersion == "" || r.Unit == "" || len(r.Scope) == 0 || r.Filters == nil || r.Cohort == nil || r.Intervals == nil ||
		r.Flags == nil || r.Conditions == nil {
		return fmt.Errorf("metric, version, unit, scope, filters, cohort, intervals, flags and conditions are required")
	}
	for i, flag := range r.Flags {
		if flag == "" || slices.Contains(r.Flags[:i], flag) {
			return fmt.Errorf("flags contain an empty or duplicate flag")
		}
	}
	for _, c := range r.Conditions {
		if (c.Op != "ge" && c.Op != "le") || !validNumber(c.Value) {
			return fmt.Errorf("condition needs op ge or le and a number")
		}
	}
	if len(r.Conditions) > 0 && r.Value != nil {
		return fmt.Errorf("a record with conditions has a null value")
	}
	if err := validateWindow(r.Period); err != nil {
		return err
	}
	asOf, err := parseSecond(r.AsOf)
	if err != nil {
		return fmt.Errorf("as_of: %w", err)
	}
	to, err := parseSecond(r.Period.To)
	if err != nil {
		return err
	}
	if asOf.Before(to) {
		return fmt.Errorf("as_of precedes period end")
	}
	if r.Step != "week" && r.Step != "month" && r.Step != "period" {
		return fmt.Errorf("step must be week, month or period")
	}
	if r.N < 0 {
		return fmt.Errorf("n must be nonnegative")
	}
	if !slices.Contains([]string{"ASSESSED", "BOUNDED", "PROVISIONAL", "UNKNOWN", "NOT_EVALUATED", "NOT_APPLICABLE"}, r.State) {
		return fmt.Errorf("invalid data state")
	}
	for _, number := range []*json.Number{r.Value, r.Numerator, r.Denominator} {
		if number != nil && !validNumber(*number) {
			return fmt.Errorf("invalid number")
		}
	}
	members := make(map[string]bool)
	for _, id := range r.Cohort {
		if id == "" || members[id] {
			return fmt.Errorf("cohort contains empty or duplicate id")
		}
		members[id] = true
	}
	for i, interval := range r.Intervals {
		if !members[interval.ObjectID] || interval.Kind == "" || !validOffset(interval.OriginalOffset) {
			return fmt.Errorf("interval %d: invalid member, kind or offset", i)
		}
		if err := validateWindow(Window{From: interval.From, To: interval.To}); err != nil {
			return fmt.Errorf("interval %d: %w", i, err)
		}
	}
	return nil
}

// Compare reports every difference of actual from expected. A record of expected with conditions is met by
// an actual value inside them; its value and conditions are then not compared as such.
func Compare(expected, actual *ExpectedFile) ([]string, error) {
	if err := expected.Validate(); err != nil {
		return nil, fmt.Errorf("expected: %w", err)
	}
	if err := actual.Validate(); err != nil {
		return nil, fmt.Errorf("actual: %w", err)
	}
	left, err := normalizedRecords(expected)
	if err != nil {
		return nil, err
	}
	right, err := normalizedRecords(actual)
	if err != nil {
		return nil, err
	}
	var differences []string
	byID := make(map[string]*Expected, len(actual.Records))
	for i := range actual.Records {
		byID[actual.Records[i].ID] = &actual.Records[i]
	}
	for i := range expected.Records {
		record := &expected.Records[i]
		got, ok := byID[record.ID]
		if len(record.Conditions) == 0 || !ok {
			continue
		}
		if !satisfies(got.Value, record.Conditions) {
			differences = append(differences, fmt.Sprintf("records/%d/value: outside conditions", i))
		}
		for _, side := range []map[string]any{left, right} {
			if fields, ok := side[record.ID].(map[string]any); ok {
				delete(fields, "value")
				delete(fields, "conditions")
			}
		}
	}
	compareJSON("records", left, right, &differences)
	return differences, nil
}

func satisfies(value *json.Number, conditions []Condition) bool {
	if value == nil {
		return false
	}
	got, ok := new(big.Rat).SetString(string(*value))
	if !ok {
		return false
	}
	for _, c := range conditions {
		bound, _ := new(big.Rat).SetString(string(c.Value))
		if (c.Op == "ge" && got.Cmp(bound) < 0) || (c.Op == "le" && got.Cmp(bound) > 0) {
			return false
		}
	}
	return true
}

func validNumber(n json.Number) bool {
	_, ok := new(big.Rat).SetString(string(n))
	return ok
}

func normalizedRecords(file *ExpectedFile) (map[string]any, error) {
	result := make(map[string]any, len(file.Records))
	for _, record := range file.Records {
		record.Cohort = slices.Clone(record.Cohort)
		sort.Strings(record.Cohort)
		data, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("encode expected record: %w", err)
		}
		var value any
		if err := Decode(data, &value); err != nil {
			return nil, err
		}
		result[record.ID] = value
	}
	return result, nil
}

func compareJSON(path string, left, right any, differences *[]string) {
	if l, ok := left.(json.Number); ok {
		r, ok := right.(json.Number)
		if ok {
			a, aok := new(big.Rat).SetString(string(l))
			b, bok := new(big.Rat).SetString(string(r))
			if aok && bok && a.Cmp(b) == 0 {
				return
			}
		}
		*differences = append(*differences, path+": different number")
		return
	}
	if l, ok := left.(map[string]any); ok {
		r, ok := right.(map[string]any)
		if !ok {
			*differences = append(*differences, path+": different type")
			return
		}
		keys := make([]string, 0, len(l)+len(r))
		for key := range l {
			keys = append(keys, key)
		}
		for key := range r {
			if _, exists := l[key]; !exists {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			lv, lok := l[key]
			rv, rok := r[key]
			// Never print user-controlled keys or values: files may contain rejected sensitive data.
			next := fmt.Sprintf("%s/field[%d]", path, sort.SearchStrings(keys, key))
			if slices.Contains([]string{"id", "metric_id", "definition_version", "scope", "filters", "period", "step", "as_of", "unit", "value", "numerator", "denominator", "n", "state", "cohort", "intervals", "object_id", "kind", "from", "to", "original_offset", "flags", "conditions", "op"}, key) {
				next = path + "/" + key
			}
			if lok != rok {
				*differences = append(*differences, next+": missing or extra field")
				continue
			}
			compareJSON(next, lv, rv, differences)
		}
		return
	}
	if l, ok := left.([]any); ok {
		r, ok := right.([]any)
		if !ok {
			*differences = append(*differences, path+": different type")
			return
		}
		for i := 0; i < max(len(l), len(r)); i++ {
			next := fmt.Sprintf("%s/%d", path, i)
			if i >= len(l) || i >= len(r) {
				*differences = append(*differences, next+": missing or extra element")
				continue
			}
			compareJSON(next, l[i], r[i], differences)
		}
		return
	}
	if !reflect.DeepEqual(left, right) {
		*differences = append(*differences, path+": different value")
	}
}

func validOffset(offset string) bool {
	if offset == "Z" {
		return true
	}
	if len(offset) != 6 || (offset[0] != '+' && offset[0] != '-') {
		return false
	}
	_, err := parseSecond("2026-01-01T00:00:00" + offset)
	return err == nil
}
