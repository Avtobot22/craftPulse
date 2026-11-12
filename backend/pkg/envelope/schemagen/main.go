// Command schemagen turns the record schemas of envelope.yaml into the JSON rules that pkg/envelope embeds:
// property order, required fields, types, enums, patterns, limits and the record kinds of each package schema.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const _refPrefix = "#/components/schemas/"

type schemaOut struct {
	Types      []string      `json:"types,omitempty"`
	Enum       []any         `json:"enum,omitempty"`
	Pattern    string        `json:"pattern,omitempty"`
	Format     string        `json:"format,omitempty"`
	MinLength  *int          `json:"min_length,omitempty"`
	MaxLength  *int          `json:"max_length,omitempty"`
	Minimum    *float64      `json:"minimum,omitempty"`
	Maximum    *float64      `json:"maximum,omitempty"`
	MinItems   *int          `json:"min_items,omitempty"`
	MaxItems   *int          `json:"max_items,omitempty"`
	Properties []propertyOut `json:"properties,omitempty"`
	Required   []string      `json:"required,omitempty"`
	Closed     bool          `json:"closed,omitempty"`
	Additional *schemaOut    `json:"additional,omitempty"`
	Items      *schemaOut    `json:"items,omitempty"`
}

type propertyOut struct {
	Name   string     `json:"name"`
	Schema *schemaOut `json:"schema"`
}

type packageOut struct {
	Versions []string `json:"versions"`
	Kinds    []string `json:"kinds"`
}

type recordOut struct {
	Kind    string          `json:"kind"`
	Version string          `json:"version"`
	Classes []string        `json:"classes"`
	Schema  *schemaOut      `json:"schema"`
	Example json.RawMessage `json:"example"`
}

type fileOut struct {
	Packages   map[string]packageOut `json:"packages"`
	Manifest   *schemaOut            `json:"manifest"`
	Record     *schemaOut            `json:"record"`
	TaskCSVRow *schemaOut            `json:"task_csv_row"`
	Records    []recordOut           `json:"records"`
}

type recordTag struct {
	Kind    string   `yaml:"kind"`
	Version string   `yaml:"version"`
	Classes []string `yaml:"classes"`
}

func main() {
	in := flag.String("in", "", "envelope.yaml")
	out := flag.String("out", "", "file to write")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	if in == "" || out == "" {
		return errors.New("-in and -out are required")
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", in, err)
	}
	root := doc.Content[0]
	schemas := child(child(root, "components"), "schemas")
	if schemas == nil {
		return errors.New("components.schemas is missing")
	}
	c := converter{schemas: schemas}
	result := fileOut{Packages: map[string]packageOut{}}
	if err := child(root, "x-craftpulse-schemas").Decode(&result.Packages); err != nil {
		return fmt.Errorf("x-craftpulse-schemas: %w", err)
	}
	for name, target := range map[string]**schemaOut{"Manifest": &result.Manifest, "Record": &result.Record, "TaskCsvRow": &result.TaskCSVRow} {
		if *target, err = c.ref(name); err != nil {
			return err
		}
	}
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		name, node := schemas.Content[i].Value, schemas.Content[i+1]
		tag := child(node, "x-craftpulse-record")
		if tag == nil {
			continue
		}
		record := recordOut{}
		var meta recordTag
		if err := tag.Decode(&meta); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		record.Kind, record.Version, record.Classes = meta.Kind, meta.Version, meta.Classes
		if record.Schema, err = c.convert(node); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if record.Example, err = example(child(node, "example")); err != nil {
			return fmt.Errorf("%s example: %w", name, err)
		}
		result.Records = append(result.Records, record)
	}
	slices.SortFunc(result.Records, func(a, b recordOut) int { return strings.Compare(a.Kind+"/"+a.Version, b.Kind+"/"+b.Version) })
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(encoded, '\n'), 0o600)
}

type converter struct {
	schemas *yaml.Node
}

func (c converter) ref(name string) (*schemaOut, error) {
	node := child(c.schemas, name)
	if node == nil {
		return nil, fmt.Errorf("schema %s is missing", name)
	}
	return c.convert(node)
}

func (c converter) convert(node *yaml.Node) (*schemaOut, error) {
	if ref := child(node, "$ref"); ref != nil {
		return c.ref(strings.TrimPrefix(ref.Value, _refPrefix))
	}
	if oneOf := child(node, "oneOf"); oneOf != nil {
		return c.nullable(oneOf)
	}
	out := &schemaOut{}
	if t := child(node, "type"); t != nil {
		if t.Kind == yaml.SequenceNode {
			for _, item := range t.Content {
				out.Types = append(out.Types, item.Value)
			}
		} else {
			out.Types = []string{t.Value}
		}
	}
	if enum := child(node, "enum"); enum != nil {
		for _, item := range enum.Content {
			var value any
			if err := item.Decode(&value); err != nil {
				return nil, err
			}
			out.Enum = append(out.Enum, value)
		}
	}
	for key, target := range map[string]*string{"pattern": &out.Pattern, "format": &out.Format} {
		if value := child(node, key); value != nil {
			*target = value.Value
		}
	}
	for key, target := range map[string]**int{"minLength": &out.MinLength, "maxLength": &out.MaxLength, "minItems": &out.MinItems, "maxItems": &out.MaxItems} {
		if err := decodeInto(child(node, key), target); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}
	for key, target := range map[string]**float64{"minimum": &out.Minimum, "maximum": &out.Maximum} {
		if err := decodeInto(child(node, key), target); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}
	if required := child(node, "required"); required != nil {
		if err := required.Decode(&out.Required); err != nil {
			return nil, err
		}
	}
	if props := child(node, "properties"); props != nil {
		for i := 0; i+1 < len(props.Content); i += 2 {
			schema, err := c.convert(props.Content[i+1])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", props.Content[i].Value, err)
			}
			out.Properties = append(out.Properties, propertyOut{Name: props.Content[i].Value, Schema: schema})
		}
	}
	if additional := child(node, "additionalProperties"); additional != nil {
		switch {
		case additional.Kind == yaml.ScalarNode && additional.Value == "false":
			out.Closed = true
		case additional.Kind == yaml.MappingNode:
			schema, err := c.convert(additional)
			if err != nil {
				return nil, err
			}
			out.Additional = schema
		}
	}
	if items := child(node, "items"); items != nil {
		schema, err := c.convert(items)
		if err != nil {
			return nil, err
		}
		out.Items = schema
	}
	return out, nil
}

// nullable supports the only union of the contract: a schema or null.
func (c converter) nullable(oneOf *yaml.Node) (*schemaOut, error) {
	if len(oneOf.Content) != 2 {
		return nil, errors.New("oneOf supports only a schema and null")
	}
	var schema *schemaOut
	nulls := 0
	for _, item := range oneOf.Content {
		if t := child(item, "type"); t != nil && t.Value == "null" {
			nulls++
			continue
		}
		converted, err := c.convert(item)
		if err != nil {
			return nil, err
		}
		schema = converted
	}
	if nulls != 1 || schema == nil {
		return nil, errors.New("oneOf supports only a schema and null")
	}
	copied := *schema
	copied.Types = append(slices.Clone(copied.Types), "null")
	return &copied, nil
}

func example(node *yaml.Node) (json.RawMessage, error) {
	if node == nil {
		return nil, errors.New("record schema needs an example")
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func decodeInto[T any](node *yaml.Node, target **T) error {
	if node == nil {
		return nil
	}
	value := new(T)
	if err := node.Decode(value); err != nil {
		return err
	}
	*target = value
	return nil
}

func child(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
