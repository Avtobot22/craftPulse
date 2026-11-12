package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
)

func ReadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON file: %w", err)
	}
	return Decode(data, target)
}

func Decode(data []byte, target any) error {
	// Reject duplicate keys before decoding; otherwise a later field silently wins.
	if err := checkKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON structure")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return requiredFields(data, reflect.TypeOf(target).Elem())
}

func checkKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON structure")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := make(map[string]bool)
	for decoder.More() {
		if delim == '{' {
			token, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("invalid object")
			}
			key, ok := token.(string)
			if !ok || keys[key] {
				return fmt.Errorf("duplicate or invalid object key")
			}
			keys[key] = true
		}
		if err := checkKeys(decoder); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("invalid JSON delimiter")
	}
	return nil
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func parseSecond(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || strings.ContainsAny(value, ".,") {
		return time.Time{}, fmt.Errorf("expected RFC3339 timestamp with whole seconds")
	}
	return parsed, nil
}

func validateWindow(window Window) error {
	from, err := parseSecond(window.From)
	if err != nil {
		return fmt.Errorf("period.from: %w", err)
	}
	to, err := parseSecond(window.To)
	if err != nil {
		return fmt.Errorf("period.to: %w", err)
	}
	if !to.After(from) {
		return fmt.Errorf("period must be nonempty and half-open")
	}
	return nil
}

func requiredFields(data []byte, shape reflect.Type) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		if shape.Kind() == reflect.Pointer || shape.Kind() == reflect.Interface {
			return nil
		}
		return fmt.Errorf("unexpected null value")
	}
	for shape.Kind() == reflect.Pointer {
		shape = shape.Elem()
	}
	if shape == reflect.TypeFor[json.Number]() {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("expected JSON number")
		}
		if _, ok := value.(json.Number); !ok {
			return fmt.Errorf("expected JSON number")
		}
		return nil
	}
	switch shape.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
			return fmt.Errorf("expected object")
		}
		allowed := make(map[string]bool, shape.NumField())
		for i := 0; i < shape.NumField(); i++ {
			allowed[strings.Split(shape.Field(i).Tag.Get("json"), ",")[0]] = true
		}
		for name := range fields {
			if !allowed[name] {
				return fmt.Errorf("unknown JSON field")
			}
		}
		for i := 0; i < shape.NumField(); i++ {
			field := shape.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			value, exists := fields[name]
			if !exists {
				return fmt.Errorf("missing required field %s", name)
			}
			if string(value) == "null" {
				if field.Type.Kind() == reflect.Pointer {
					continue
				}
				return fmt.Errorf("field %s cannot be null", name)
			}
			if err := requiredFields(value, field.Type); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case reflect.Slice:
		var elements []json.RawMessage
		if err := json.Unmarshal(data, &elements); err != nil {
			return fmt.Errorf("expected array")
		}
		for _, element := range elements {
			if err := requiredFields(element, shape.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return fmt.Errorf("expected map")
		}
		for _, value := range fields {
			if err := requiredFields(value, shape.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}
