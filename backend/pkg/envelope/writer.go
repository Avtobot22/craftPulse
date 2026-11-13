package envelope

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ErrWriterClosed is returned when a record or file is added after Close.
var ErrWriterClosed = errors.New("package writer is closed")

// Writer builds a package: the manifest first, then files, then records.jsonl on Close. Headers carry a
// fixed time and mode, so the same content always gives the same bytes and the same package hash.
type Writer struct {
	gz      *gzip.Writer
	tw      *tar.Writer
	records bytes.Buffer
	csv     bool
	spooled bool
	closed  bool
}

func NewWriter(w io.Writer, manifest *Manifest) (*Writer, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	gz := gzip.NewWriter(w)
	pw := &Writer{gz: gz, tw: tar.NewWriter(gz)}
	if err := pw.entry(ManifestName, data); err != nil {
		return nil, err
	}
	return pw, nil
}

// AddFile stores content under files/ and returns the reference for a record.
func (w *Writer) AddFile(name, mediaType string, content []byte) (FileRef, error) {
	if w.closed {
		return FileRef{}, ErrWriterClosed
	}
	if !strings.HasPrefix(name, FilesDir) || !safePath(name) {
		return FileRef{}, fmt.Errorf("file path must be a clean path under %s", FilesDir)
	}
	if err := w.entry(name, content); err != nil {
		return FileRef{}, err
	}
	sum := sha256.Sum256(content)
	return FileRef{Path: name, Sha256: hex.EncodeToString(sum[:]), SizeBytes: len(content), MediaType: mediaType}, nil
}

// AddFileFrom streams size bytes of r under files/ without holding the content in memory.
func (w *Writer) AddFileFrom(name, mediaType string, r io.Reader, size int64) (FileRef, error) {
	if w.closed {
		return FileRef{}, ErrWriterClosed
	}
	if !strings.HasPrefix(name, FilesDir) || !safePath(name) {
		return FileRef{}, fmt.Errorf("file path must be a clean path under %s", FilesDir)
	}
	h := sha256.New()
	if err := w.entryFrom(name, io.TeeReader(r, h), size); err != nil {
		return FileRef{}, err
	}
	return FileRef{Path: name, Sha256: hex.EncodeToString(h.Sum(nil)), SizeBytes: int(size), MediaType: mediaType}, nil
}

// AddRecordsFrom streams a ready records.jsonl of size bytes in place of records added one by one; the
// package is finished by Close.
func (w *Writer) AddRecordsFrom(r io.Reader, size int64) error {
	if w.closed {
		return ErrWriterClosed
	}
	if w.csv || w.spooled || w.records.Len() > 0 {
		return errors.New("package already has records")
	}
	w.spooled = true
	return w.entryFrom(RecordsName, r, size)
}

// AddRecord appends one line to records.jsonl.
func (w *Writer) AddRecord(rec *Record) error {
	if w.closed {
		return ErrWriterClosed
	}
	if w.csv {
		return errors.New("package already has a task CSV")
	}
	if w.spooled {
		return errors.New("package already has records")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	w.records.Write(data)
	w.records.WriteByte('\n')
	return nil
}

// AddTasksCSV stores a current-state task CSV instead of records.jsonl; the manifest must be a tasks snapshot.
func (w *Writer) AddTasksCSV(content []byte) error {
	if w.closed {
		return ErrWriterClosed
	}
	if w.spooled || w.records.Len() > 0 {
		return errors.New("package already has records")
	}
	w.csv = true
	return w.entry(TasksCSVName, content)
}

// Close writes records.jsonl and finishes the archive; it does not close the underlying writer.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if !w.csv && !w.spooled {
		if err := w.entry(RecordsName, w.records.Bytes()); err != nil {
			return err
		}
	}
	if err := w.tw.Close(); err != nil {
		return fmt.Errorf("finish tar: %w", err)
	}
	if err := w.gz.Close(); err != nil {
		return fmt.Errorf("finish gzip: %w", err)
	}
	return nil
}

func (w *Writer) entry(name string, content []byte) error {
	return w.entryFrom(name, bytes.NewReader(content), int64(len(content)))
}

func (w *Writer) entryFrom(name string, r io.Reader, size int64) error {
	header := &tar.Header{Name: name, Mode: 0o644, Size: size, ModTime: time.Unix(0, 0).UTC(), Typeflag: tar.TypeReg, Format: tar.FormatPAX}
	if err := w.tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write %s header: %w", name, err)
	}
	n, err := io.Copy(w.tw, r)
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if n != size {
		return fmt.Errorf("write %s: got %d bytes, want %d", name, n, size)
	}
	return nil
}
