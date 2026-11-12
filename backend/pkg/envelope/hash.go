package envelope

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ErrUnknownPayloadSchema is returned for a payload_schema that envelope.yaml does not declare.
var ErrUnknownPayloadSchema = errors.New("unknown payload schema")

// ContentHash is the SHA-256 of the canonical content of a record (TZ 12.6): the event time, the payload
// fields in the order of their schema and the file hash. Envelope service fields - class, kind, schema,
// native ns and id, source revision, observed time and file path - do not take part. A null field equals
// an absent one, keys of free-form objects are sorted.
func ContentHash(rec *Record) (string, error) {
	rule, ok := _rules.record(rec.PayloadSchema)
	if !ok {
		return "", ErrUnknownPayloadSchema
	}
	payload, err := decode(rec.Payload)
	if err != nil {
		return "", fmt.Errorf("decode payload: %w", err)
	}
	var eventTime *string
	if rec.EventTime != nil {
		formatted := rec.EventTime.Format(time.RFC3339Nano)
		eventTime = &formatted
	}
	var fileSHA string
	if rec.File != nil {
		fileSHA = rec.File.Sha256
	}
	return hashContent(eventTime, rule.Schema, payload, fileSHA)
}

func hashContent(eventTime *string, s *schema, payload any, fileSHA string) (string, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	if eventTime != nil {
		b.WriteString(`"event_time":`)
		if err := canonical(&b, &schema{Format: "date-time"}, *eventTime); err != nil {
			return "", err
		}
		b.WriteByte(',')
	}
	b.WriteString(`"payload":`)
	if err := canonical(&b, s, payload); err != nil {
		return "", err
	}
	if fileSHA != "" {
		b.WriteString(`,"file_sha256":`)
		writeString(&b, fileSHA)
	}
	b.WriteByte('}')
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func canonical(b *bytes.Buffer, s *schema, value any) error {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(v))
	case string:
		if s != nil && s.Format == "date-time" {
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				v = t.Format(time.RFC3339Nano)
			}
		}
		writeString(b, v)
	case json.Number:
		return canonicalNumber(b, s, v)
	case []any:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := canonical(b, itemSchema(s), item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		return canonicalObject(b, s, v)
	default:
		return fmt.Errorf("unsupported JSON value %T", value)
	}
	return nil
}

// canonicalObject writes declared fields in schema order, then other keys sorted; nulls are skipped.
func canonicalObject(b *bytes.Buffer, s *schema, v map[string]any) error {
	b.WriteByte('{')
	first := true
	field := func(name string, fs *schema) error {
		value, ok := v[name]
		if !ok || value == nil {
			return nil
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		writeString(b, name)
		b.WriteByte(':')
		return canonical(b, fs, value)
	}
	declared := map[string]bool{}
	if s != nil {
		for _, p := range s.Properties {
			declared[p.Name] = true
			if err := field(p.Name, p.Schema); err != nil {
				return err
			}
		}
	}
	for _, name := range sortedKeys(v) {
		if declared[name] {
			continue
		}
		var extra *schema
		if s != nil {
			extra = s.Additional
		}
		if err := field(name, extra); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

func canonicalNumber(b *bytes.Buffer, s *schema, v json.Number) error {
	if i, err := v.Int64(); err == nil {
		b.WriteString(strconv.FormatInt(i, 10))
		return nil
	}
	f, err := v.Float64()
	if err != nil {
		return fmt.Errorf("invalid number: %w", err)
	}
	if s != nil && len(s.Types) > 0 && s.Types[0] == "integer" && f == float64(int64(f)) {
		b.WriteString(strconv.FormatInt(int64(f), 10))
		return nil
	}
	b.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	return nil
}

func itemSchema(s *schema) *schema {
	if s == nil {
		return nil
	}
	return s.Items
}

func writeString(b *bytes.Buffer, v string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // encoding a string into a buffer cannot fail
	b.Truncate(b.Len() - 1)
}

func decode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data after JSON value")
	}
	return v, nil
}
