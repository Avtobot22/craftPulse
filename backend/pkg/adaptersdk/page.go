package adaptersdk

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// ErrPageFull means the page reached the package limits: the source ends the page here and puts the
// position of the record it did not add into the next cursor.
var ErrPageFull = errors.New("adaptersdk: page reached the package limits")

// Page collects one Fetch page on disk, because the manifest with the next cursor is known only after
// the last record and goes first in the package. Memory stays bounded whatever the page size (TZ 14.9).
type Page struct {
	dir      string
	limits   envelope.Limits
	records  *os.File
	buf      *bufio.Writer
	size     int64
	count    int
	contents *os.File
	files    []pageFile
	unpacked int64
	wrap     func(io.Writer) io.Writer
}

type pageFile struct {
	name, mediaType string
	offset, size    int64
}

// _manifestReserve keeps room for the manifest in the unpacked limit.
const _manifestReserve = 1 << 20

func newPage(dir string, limits envelope.Limits) *Page {
	limits.MaxUnpackedBytes -= _manifestReserve
	return &Page{dir: dir, limits: limits}
}

// Len is the number of records added.
func (p *Page) Len() int { return p.count }

func (p *Page) AddRecord(rec *envelope.Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("adaptersdk: encode record: %w", err)
	}
	if err := p.reserve(int64(len(data)) + 1); err != nil {
		return err
	}
	if p.records == nil {
		if p.records, err = os.CreateTemp(p.dir, "page-records-*"); err != nil {
			return fmt.Errorf("adaptersdk: spool records: %w", err)
		}
		p.buf = bufio.NewWriterSize(p.records, 64<<10)
	}
	if _, err := p.buf.Write(data); err != nil {
		return fmt.Errorf("adaptersdk: spool records: %w", err)
	}
	if err := p.buf.WriteByte('\n'); err != nil {
		return fmt.Errorf("adaptersdk: spool records: %w", err)
	}
	p.size += int64(len(data)) + 1
	p.count++
	return nil
}

// AddFile stores a report file under files/ and returns the reference for its record.
func (p *Page) AddFile(name, mediaType string, r io.Reader) (envelope.FileRef, error) {
	if !strings.HasPrefix(name, envelope.FilesDir) {
		return envelope.FileRef{}, fmt.Errorf("adaptersdk: file path must be under %s", envelope.FilesDir)
	}
	// The package also holds the manifest and records.jsonl.
	if len(p.files)+3 > p.limits.MaxFiles {
		return envelope.FileRef{}, ErrPageFull
	}
	if p.contents == nil {
		var err error
		if p.contents, err = os.CreateTemp(p.dir, "page-files-*"); err != nil {
			return envelope.FileRef{}, fmt.Errorf("adaptersdk: spool file: %w", err)
		}
	}
	offset, err := p.contents.Seek(0, io.SeekEnd)
	if err != nil {
		return envelope.FileRef{}, fmt.Errorf("adaptersdk: spool file: %w", err)
	}
	h := sha256.New()
	left := p.limits.MaxUnpackedBytes - p.unpacked
	n, err := io.Copy(io.MultiWriter(p.contents, h), io.LimitReader(r, left+1))
	if err != nil {
		return envelope.FileRef{}, fmt.Errorf("adaptersdk: spool file: %w", err)
	}
	if n > left {
		_ = p.contents.Truncate(offset)
		return envelope.FileRef{}, ErrPageFull
	}
	p.unpacked += n
	p.files = append(p.files, pageFile{name: name, mediaType: mediaType, offset: offset, size: n})
	return envelope.FileRef{Path: name, Sha256: hex.EncodeToString(h.Sum(nil)), SizeBytes: int(n), MediaType: mediaType}, nil
}

// WrapOutput replaces the response stream of the page with wrap(stream), so a test adapter can cut or
// break the package while it is written. Built-in adapters do not use it.
func (p *Page) WrapOutput(wrap func(io.Writer) io.Writer) { p.wrap = wrap }

func (p *Page) reserve(n int64) error {
	if p.unpacked+n > p.limits.MaxUnpackedBytes {
		return ErrPageFull
	}
	p.unpacked += n
	return nil
}

// writeTo streams the package: the manifest, the files, then records.jsonl.
func (p *Page) writeTo(w io.Writer, manifest *envelope.Manifest) error {
	if p.buf != nil {
		if err := p.buf.Flush(); err != nil {
			return fmt.Errorf("adaptersdk: spool records: %w", err)
		}
	}
	if p.wrap != nil {
		w = p.wrap(w)
	}
	pw, err := envelope.NewWriter(w, manifest)
	if err != nil {
		return err
	}
	for _, f := range p.files {
		if _, err := pw.AddFileFrom(f.name, f.mediaType, io.NewSectionReader(p.contents, f.offset, f.size), f.size); err != nil {
			return err
		}
	}
	var records io.Reader = strings.NewReader("")
	if p.records != nil {
		records = io.NewSectionReader(p.records, 0, p.size)
	}
	if err := pw.AddRecordsFrom(records, p.size); err != nil {
		return err
	}
	return pw.Close()
}

// Close removes the spooled data.
func (p *Page) Close() error {
	var errs []error
	for _, f := range []*os.File{p.records, p.contents} {
		if f == nil {
			continue
		}
		errs = append(errs, f.Close(), os.Remove(f.Name()))
	}
	return errors.Join(errs...)
}
