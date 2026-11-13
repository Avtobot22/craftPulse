package envelope

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"slices"
)

// Clean checks a package and writes the cleaned one to w: the manifest, every record that passed on its
// own line number - a rejected or empty line stays an empty one, so record numbers of the journal hold
// in the cleaned package - and only the files those records reference. A task CSV becomes snapshot
// records. The same input always gives the same bytes. Nothing is written when the whole package is
// rejected; a stream that breaks while it is read is rejected as an invalid archive.
func Clean(r io.Reader, limits Limits, w io.Writer) (*Result, error) {
	data, err := io.ReadAll(io.LimitReader(r, limits.MaxPackageBytes+1))
	switch {
	case err != nil:
		return &Result{Envelope: &Rejection{ReasonInvalidArchive, "stream ended with an error"}}, nil //nolint:nilerr // a broken stream is a rejected package
	case int64(len(data)) > limits.MaxPackageBytes:
		return &Result{Envelope: &Rejection{ReasonLimitExceeded, "package is larger than the limit"}}, nil
	}
	c := &checker{limits: limits, files: map[string]fileEntry{}, entries: map[string]bool{}, lines: map[int][]byte{}}
	if rejection := c.read(bytes.NewReader(data)); rejection != nil {
		return &Result{Manifest: c.result.Manifest, Envelope: rejection}, nil //nolint:nilerr // the rejection is the outcome of the check
	}
	c.checkFiles()
	if err := c.write(data, w); err != nil {
		return nil, err
	}
	return &c.result, nil
}

func (c *checker) write(data []byte, w io.Writer) error {
	last := 0
	wanted := map[string]bool{}
	for _, rec := range c.result.Records {
		if rec.Rejection == nil {
			last = max(last, rec.No)
		}
	}
	for _, ref := range c.fileRefs {
		if c.result.Records[ref.result].Rejection == nil {
			wanted[ref.path] = true
		}
	}
	pw, err := NewWriter(w, c.result.Manifest)
	if err != nil {
		return err
	}
	if err := copyFiles(data, wanted, pw); err != nil {
		return err
	}
	var records bytes.Buffer
	accepted := map[int]bool{}
	for _, rec := range c.result.Records {
		accepted[rec.No] = rec.Rejection == nil
	}
	for no := 1; no <= last; no++ {
		if accepted[no] {
			records.Write(c.lines[no])
		}
		records.WriteByte('\n')
	}
	if err := pw.AddRecordsFrom(bytes.NewReader(records.Bytes()), int64(records.Len())); err != nil {
		return err
	}
	return pw.Close()
}

// copyFiles streams the wanted files of the checked archive in the order they appear in it.
func copyFiles(data []byte, wanted map[string]bool, pw *Writer) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("reopen package: %w", err)
	}
	tr := tar.NewReader(gz)
	names := make([]string, 0, len(wanted))
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reread package: %w", err)
		}
		if !wanted[header.Name] || slices.Contains(names, header.Name) {
			continue
		}
		names = append(names, header.Name)
		if _, err := pw.AddFileFrom(header.Name, "", tr, header.Size); err != nil {
			return err
		}
	}
}
