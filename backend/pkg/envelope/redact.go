package envelope

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Redactor discovers values across a package before replacing any of its text.
type Redactor interface {
	Discover(name string, content []byte) error
	Redact(name string, content []byte) (sanitized []byte, replacements int64, err error)
}

// CleanWithRedactor validates input before discovery and revalidates rewritten file references and records.
func CleanWithRedactor(r io.Reader, limits Limits, w io.Writer, redactor Redactor) (*Result, error) {
	return cleanPackage(r, limits, w, redactor, false)
}

// CleanPageWithRedactor cleans a page of an adapter like CleanWithRedactor; its report files are artifacts the
// adapter read from the CI of the source, so they may come without a scan manifest.
func CleanPageWithRedactor(r io.Reader, limits Limits, w io.Writer, redactor Redactor) (*Result, error) {
	return cleanPackage(r, limits, w, redactor, true)
}

func rejectedRedaction(err error) (*Result, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	return &Result{Envelope: &Rejection{ReasonInvalidArchive, "package could not be safely redacted"}}, nil
}

func redactRejected(res *Result, redactor Redactor) (*Result, error) {
	if redactor == nil || res.Manifest == nil {
		return res, nil
	}
	raw, err := json.Marshal(res.Manifest)
	if err != nil {
		return rejectedRedaction(err)
	}
	if err := redactor.Discover(ManifestName, raw); err != nil {
		return rejectedRedaction(err)
	}
	safe, count, err := redactor.Redact(ManifestName, raw)
	if err != nil {
		return rejectedRedaction(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(safe, &manifest); err != nil {
		return rejectedRedaction(err)
	}
	res.Manifest = &manifest
	res.Redactions = count
	res.Envelope.Detail = "package failed validation"
	return res, nil
}

func (c *checker) redact(data []byte, w io.Writer, redactor Redactor) (*Result, error) {
	if err := walkArchive(data, func(name string, content []byte) error {
		if err := redactor.Discover(name, []byte(name)); err != nil {
			return err
		}
		return redactor.Discover(name, content)
	}); err != nil {
		return rejectedRedaction(err)
	}
	var checked bytes.Buffer
	if err := c.write(data, c.result.Manifest, &checked); err != nil {
		return nil, err
	}
	rewritten, count, err := redactArchive(checked.Bytes(), redactor)
	if err != nil {
		return rejectedRedaction(err)
	}
	var safe bytes.Buffer
	result, err := cleanPackage(bytes.NewReader(rewritten), c.limits, &safe, nil, c.page)
	if err != nil {
		return nil, err
	}
	if result.Envelope != nil {
		return redactRejected(result, redactor)
	}
	byNumber := map[int]RecordResult{}
	for _, record := range result.Records {
		byNumber[record.No] = record
	}
	records := make([]RecordResult, 0, len(c.result.Records))
	for _, original := range c.result.Records {
		if original.Rejection == nil {
			records = append(records, byNumber[original.No])
			continue
		}
		raw, err := json.Marshal(original)
		if err != nil {
			return rejectedRedaction(err)
		}
		sanitized, n, err := redactor.Redact(RecordsName, raw)
		if err != nil {
			return rejectedRedaction(err)
		}
		if err := json.Unmarshal(sanitized, &original); err != nil {
			return rejectedRedaction(err)
		}
		count += n
		records = append(records, original)
	}
	result.Records, result.Redactions = records, count
	if verifier, ok := redactor.(interface{ VerifyPackage([]byte) error }); ok {
		if err := verifier.VerifyPackage(safe.Bytes()); err != nil {
			return rejectedRedaction(err)
		}
	}
	if _, err := w.Write(safe.Bytes()); err != nil {
		return nil, err
	}
	return result, nil
}

// walkArchive only receives archives already checked against package, path and unpacked-size limits.
func walkArchive(data []byte, visit func(string, []byte) error) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
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
		if err := visit(h.Name, content); err != nil {
			return err
		}
	}
}

func redactArchive(data []byte, redactor Redactor) (sanitized []byte, replacements int64, err error) {
	refs := map[string]FileRef{}
	oldHashes := map[string]string{}
	// Hash each rewritten file without retaining unpacked files in memory.
	if err := walkArchive(data, func(name string, content []byte) error {
		if !strings.HasPrefix(name, FilesDir) {
			return nil
		}
		safe, _, err := redactor.Redact(name, content)
		if err != nil {
			return err
		}
		safeName, _, err := redactor.Redact("", []byte(name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(safe)
		ref := FileRef{Path: string(safeName), Sha256: hex.EncodeToString(sum[:]), SizeBytes: len(safe)}
		refs[name] = ref
		refs[ref.Path] = ref
		return nil
	}); err != nil {
		return nil, 0, err
	}
	var out bytes.Buffer
	var writer *Writer
	var count int64
	err = walkArchive(data, func(name string, content []byte) error {
		safe, n, err := redactor.Redact(name, content)
		if err != nil {
			return err
		}
		count += n
		if strings.HasPrefix(name, FilesDir) {
			ref := refs[name]
			_, err := writer.AddFile(ref.Path, ref.MediaType, safe)
			return err
		}
		if name == RecordsName {
			var lines bytes.Buffer
			for _, line := range bytes.Split(bytes.TrimSuffix(safe, []byte{'\n'}), []byte{'\n'}) {
				if len(bytes.TrimSpace(line)) > 0 {
					updated, err := rewriteReferences(line, refs, oldHashes)
					if err != nil {
						return err
					}
					lines.Write(updated)
				}
				lines.WriteByte('\n')
			}
			return writer.AddRecordsFrom(&lines, int64(lines.Len()))
		}
		safe, err = rewriteReferences(safe, refs, oldHashes)
		if err != nil {
			return err
		}
		switch name {
		case ManifestName:
			var manifest Manifest
			if err := json.Unmarshal(safe, &manifest); err != nil {
				return err
			}
			writer, err = NewWriter(&out, &manifest)
			return err
		case ScanManifestName, ExecutionManifestName:
			return writer.entry(name, safe)
		}
		return errors.New("unexpected cleaned package entry")
	})
	if err != nil {
		return nil, 0, err
	}
	if err := writer.Close(); err != nil {
		return nil, 0, err
	}
	return out.Bytes(), count, nil
}

func rewriteReferences(data []byte, refs map[string]FileRef, oldHashes map[string]string) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("cleaned metadata is not JSON")
	}
	var visit func(any)
	visit = func(value any) {
		switch node := value.(type) {
		case []any:
			for _, item := range node {
				visit(item)
			}
		case map[string]any:
			path, _ := node["path"].(string)
			if old, hasHash := node["sha256"].(string); hasHash {
				ref, ok := refs[path]
				relative := !strings.HasPrefix(path, FilesDir)
				if !ok && relative {
					ref, ok = refs[FilesDir+path]
				}
				if ok {
					oldHashes[old] = ref.Sha256
					node["sha256"], node["path"] = ref.Sha256, ref.Path
					if relative {
						node["path"] = strings.TrimPrefix(ref.Path, FilesDir)
					}
					if _, hasSize := node["size_bytes"]; hasSize {
						node["size_bytes"] = ref.SizeBytes
					}
				}
			}
			for _, item := range node {
				visit(item)
			}
			if node["record_kind"] == "report_file" {
				if old, ok := node["source_revision"].(string); ok {
					if sha, ok := oldHashes[old]; ok {
						node["source_revision"] = sha
					}
				}
			}
		}
	}
	visit(value)
	return json.Marshal(value)
}
