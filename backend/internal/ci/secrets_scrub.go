package ci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// SecretScrubber keeps secret fragments only in memory for the duration of one command.
// Every eight-byte window is removed, including occurrences in other tools' reports.
type SecretScrubber struct {
	min     int
	windows map[uint64]struct{}
	short   []string
}

func NewSecretScrubber(minLength int) *SecretScrubber {
	return &SecretScrubber{min: minLength, windows: map[uint64]struct{}{}}
}

func (s *SecretScrubber) add(value string) {
	if value == "" {
		return
	}
	if len(value) < s.min {
		s.short = append(s.short, value)
		return
	}
	for i := 0; i+s.min <= len(value); i++ {
		s.windows[binary.LittleEndian.Uint64([]byte(value[i:i+8]))] = struct{}{}
	}
}

func (s *SecretScrubber) clean(data []byte) []byte {
	mask := make([]bool, len(data))
	for i := 0; i+8 <= len(data); i++ {
		if _, ok := s.windows[binary.LittleEndian.Uint64(data[i:i+8])]; ok {
			for j := i; j < i+8; j++ {
				mask[j] = true
			}
		}
	}
	for _, short := range s.short {
		for start := 0; start < len(data); {
			at := bytes.Index(data[start:], []byte(short))
			if at < 0 {
				break
			}
			at += start
			for j := at; j < at+len(short); j++ {
				mask[j] = true
			}
			start = at + 1
		}
	}
	var out bytes.Buffer
	for i := 0; i < len(data); {
		if !mask[i] {
			out.WriteByte(data[i])
			i++
			continue
		}
		out.WriteString("[!]")
		for i < len(data) && mask[i] {
			i++
		}
	}
	return out.Bytes()
}
func (s *SecretScrubber) contains(data []byte) bool { return !bytes.Equal(data, s.clean(data)) }

// Scrub decodes text strings before redaction so JSON/XML escaping cannot preserve a value.
// It preserves the report syntax, then checks the serialized text again.
func (s *SecretScrubber) Scrub(_ *envelope.ScanReport, content []byte) ([]byte, error) {
	// UTF-8 BOMs are common in JUnit from Windows runners; do not let them bypass parsing.
	content = bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})
	data := content
	if json.Valid(content) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("cannot decode report for secret redaction")
		}
		data, _ = json.Marshal(s.jsonValue(value))
	} else if strings.HasPrefix(strings.TrimSpace(string(content)), "<") {
		decoder := xml.NewDecoder(bytes.NewReader(content))
		var out bytes.Buffer
		encoder := xml.NewEncoder(&out)
		for {
			token, err := decoder.RawToken()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, errors.New("cannot decode XML for secret redaction")
			}
			switch v := token.(type) {
			case xml.CharData:
				token = xml.CharData(s.clean(v))
			case xml.Comment:
				token = xml.Comment(s.clean(v))
			case xml.Directive:
				token = xml.Directive(s.clean(v))
			case xml.ProcInst:
				v.Inst = s.clean(v.Inst)
				v.Target = string(s.clean([]byte(v.Target)))
				token = v
			case xml.StartElement:
				v.Name = s.xmlName(v.Name)
				for i := range v.Attr {
					v.Attr[i].Name = s.xmlName(v.Attr[i].Name)
					v.Attr[i].Value = string(s.clean([]byte(v.Attr[i].Value)))
				}
				token = v
			case xml.EndElement:
				v.Name = s.xmlName(v.Name)
				token = v
			}
			if err := encoder.EncodeToken(token); err != nil {
				return nil, errors.New("cannot encode redacted XML")
			}
		}
		if err := encoder.Close(); err != nil {
			return nil, errors.New("cannot finish redacted XML")
		}
		data = out.Bytes()
	}
	for i := 0; i < 4 && s.contains(data); i++ {
		data = s.clean(data)
	}
	if s.contains(data) {
		return nil, errors.New("secret redaction could not eliminate all fragments")
	}
	return data, nil
}

func (s *SecretScrubber) xmlName(n xml.Name) xml.Name {
	n.Space = string(s.clean([]byte(n.Space)))
	n.Local = string(s.clean([]byte(n.Local)))
	// RawToken preserves namespace prefixes, including declarations and QName attribute values.
	// Encode the original qualified name literally rather than synthesizing duplicate xmlns attributes.
	if n.Space != "" {
		n.Local = n.Space + ":" + n.Local
		n.Space = ""
	}
	return n
}

func (s *SecretScrubber) jsonValue(v any) any {
	switch x := v.(type) {
	case string:
		return string(s.clean([]byte(x)))
	case []any:
		for i := range x {
			x[i] = s.jsonValue(x[i])
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[string(s.clean([]byte(k)))] = s.jsonValue(v)
		}
		return out
	}
	return v
}

// VerifyPackage checks every uncompressed entry, including metadata and archive paths, before upload.
func (s *SecretScrubber) VerifyPackage(data []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		if s.contains([]byte(header.Name)) || s.contains(content) {
			return errors.New("package contains secret fragments")
		}
		safe, err := s.Scrub(nil, content)
		if err != nil {
			return err
		}
		// Escaped representations must be safe too; serialization alone need not be byte identical.
		if json.Valid(content) {
			var before, after any
			if json.Unmarshal(content, &before) != nil || json.Unmarshal(safe, &after) != nil {
				return errors.New("invalid redacted package JSON")
			}
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(after)
			if !bytes.Equal(a, b) {
				return errors.New("package contains escaped secret fragments")
			}
		}
	}
}
