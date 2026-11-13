package envelope

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"
)

//go:embed schemas.gen.json
var _schemasJSON []byte

var _rules = mustRules()

// schema is the subset of JSON Schema that envelope.yaml uses, with properties in declaration order.
type schema struct {
	Types      []string   `json:"types"`
	Enum       []any      `json:"enum"`
	Pattern    string     `json:"pattern"`
	Format     string     `json:"format"`
	MinLength  *int       `json:"min_length"`
	MaxLength  *int       `json:"max_length"`
	Minimum    *float64   `json:"minimum"`
	Maximum    *float64   `json:"maximum"`
	MinItems   *int       `json:"min_items"`
	MaxItems   *int       `json:"max_items"`
	Properties []property `json:"properties"`
	Required   []string   `json:"required"`
	Closed     bool       `json:"closed"`
	Additional *schema    `json:"additional"`
	Items      *schema    `json:"items"`

	re *regexp.Regexp
}

type property struct {
	Name   string  `json:"name"`
	Schema *schema `json:"schema"`
}

type packageRule struct {
	Versions []string `json:"versions"`
	Kinds    []string `json:"kinds"`
}

type recordRule struct {
	Kind    string          `json:"kind"`
	Version string          `json:"version"`
	Classes []string        `json:"classes"`
	Schema  *schema         `json:"schema"`
	Example json.RawMessage `json:"example"`
}

type rules struct {
	Packages   map[string]packageRule `json:"packages"`
	Manifest   *schema                `json:"manifest"`
	Record     *schema                `json:"record"`
	TaskCSVRow *schema                `json:"task_csv_row"`
	Records    []recordRule           `json:"records"`
}

// mustRules panics only if the generated schemas.gen.json is broken, which the package tests catch.
func mustRules() *rules {
	var r rules
	if err := json.Unmarshal(_schemasJSON, &r); err != nil {
		panic(fmt.Sprintf("envelope: invalid schemas.gen.json: %v", err))
	}
	for _, s := range append([]*schema{r.Manifest, r.Record, r.TaskCSVRow}, schemaList(r.Records)...) {
		if err := s.compile(); err != nil {
			panic(fmt.Sprintf("envelope: %v", err))
		}
	}
	return &r
}

func schemaList(records []recordRule) []*schema {
	out := make([]*schema, len(records))
	for i := range records {
		out[i] = records[i].Schema
	}
	return out
}

func (r *rules) record(payloadSchema string) (recordRule, bool) {
	i := slices.IndexFunc(r.Records, func(rule recordRule) bool { return rule.Kind+"/"+rule.Version == payloadSchema })
	if i < 0 {
		return recordRule{}, false
	}
	return r.Records[i], true
}

func (s *schema) compile() error {
	if s == nil {
		return nil
	}
	if s.Pattern != "" {
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			return fmt.Errorf("pattern %q: %w", s.Pattern, err)
		}
		s.re = re
	}
	for _, p := range s.Properties {
		if err := p.Schema.compile(); err != nil {
			return err
		}
	}
	if err := s.Additional.compile(); err != nil {
		return err
	}
	return s.Items.compile()
}

func (s *schema) property(name string) *schema {
	for _, p := range s.Properties {
		if p.Name == name {
			return p.Schema
		}
	}
	return nil
}

// violation names the first field that does not match the schema.
type violation struct {
	path, problem string
}

func (v *violation) Error() string {
	if v.path == "" {
		return v.problem
	}
	return v.path + ": " + v.problem
}

// validate checks a value decoded with json.Decoder.UseNumber.
func (s *schema) validate(value any, path string) *violation {
	if s == nil {
		return nil
	}
	if !s.typeMatches(value) {
		return &violation{path, fmt.Sprintf("expected %v", s.Types)}
	}
	if value == nil {
		return nil
	}
	if len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(e any) bool { return e == value }) {
		return &violation{path, "value is not in the enumeration"}
	}
	switch v := value.(type) {
	case string:
		return s.validateString(v, path)
	case json.Number:
		return s.validateNumber(v, path)
	case []any:
		return s.validateArray(v, path)
	case map[string]any:
		return s.validateObject(v, path)
	}
	return nil
}

func (s *schema) typeMatches(value any) bool {
	if len(s.Types) == 0 {
		return true
	}
	for _, t := range s.Types {
		if jsonType(value, t) {
			return true
		}
	}
	return false
}

func jsonType(value any, t string) bool {
	switch v := value.(type) {
	case nil:
		return t == "null"
	case string:
		return t == "string"
	case bool:
		return t == "boolean"
	case json.Number:
		if t == "number" {
			return true
		}
		_, err := v.Int64()
		return t == "integer" && err == nil
	case []any:
		return t == "array"
	case map[string]any:
		return t == "object"
	}
	return false
}

func (s *schema) validateString(v, path string) *violation {
	length := utf8.RuneCountInString(v)
	switch {
	case s.MinLength != nil && length < *s.MinLength:
		return &violation{path, "string is too short"}
	case s.MaxLength != nil && length > *s.MaxLength:
		return &violation{path, "string is too long"}
	case s.re != nil && !s.re.MatchString(v):
		return &violation{path, "string does not match the pattern"}
	case s.Format == "date-time":
		if _, err := time.Parse(time.RFC3339Nano, v); err != nil {
			return &violation{path, "expected RFC 3339 date-time with offset"}
		}
	}
	return nil
}

func (s *schema) validateNumber(v json.Number, path string) *violation {
	f, err := strconv.ParseFloat(string(v), 64)
	switch {
	case err != nil:
		return &violation{path, "invalid number"}
	case s.Minimum != nil && f < *s.Minimum:
		return &violation{path, "number is below the minimum"}
	case s.Maximum != nil && f > *s.Maximum:
		return &violation{path, "number is above the maximum"}
	}
	return nil
}

func (s *schema) validateArray(v []any, path string) *violation {
	if s.MinItems != nil && len(v) < *s.MinItems {
		return &violation{path, "too few items"}
	}
	if s.MaxItems != nil && len(v) > *s.MaxItems {
		return &violation{path, "too many items"}
	}
	for i, item := range v {
		if bad := s.Items.validate(item, fmt.Sprintf("%s[%d]", path, i)); bad != nil {
			return bad
		}
	}
	return nil
}

func (s *schema) validateObject(v map[string]any, path string) *violation {
	for _, name := range s.Required {
		if _, ok := v[name]; !ok {
			return &violation{join(path, name), "required field is missing"}
		}
	}
	for _, p := range s.Properties {
		if value, ok := v[p.Name]; ok {
			if bad := p.Schema.validate(value, join(path, p.Name)); bad != nil {
				return bad
			}
		}
	}
	for _, name := range sortedKeys(v) {
		if s.property(name) != nil {
			continue
		}
		if s.Closed {
			return &violation{join(path, name), "unknown field"}
		}
		if bad := s.Additional.validate(v[name], join(path, name)); bad != nil {
			return bad
		}
	}
	return nil
}

// text reads a string field that a schema has already validated; anything else reads as empty.
func text(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// RecordSchema describes a record schema of envelope.yaml: kind, version, allowed classes and its example
// payload.
type RecordSchema struct {
	Kind    string
	Version string
	Classes []string
	Example json.RawMessage
}

// RecordSchemas lists every record schema in kind and version order.
func RecordSchemas() []RecordSchema {
	out := make([]RecordSchema, len(_rules.Records))
	for i, r := range _rules.Records {
		out[i] = RecordSchema{Kind: r.Kind, Version: r.Version, Classes: slices.Clone(r.Classes), Example: slices.Clone(r.Example)}
	}
	return out
}

// PackageKinds lists the record kinds a package of schemaID may contain; nil for an unknown schema.
func PackageKinds(schemaID string) []string {
	return slices.Clone(_rules.Packages[schemaID].Kinds)
}
