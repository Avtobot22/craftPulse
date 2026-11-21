package envelope

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var errNoManifest = errors.New("envelope: cleaned package has no manifest")

// Package is a cleaned package read back for normalization: the manifest and the record of every number.
type Package struct {
	Manifest          *Manifest
	ScanManifest      *ScanManifestV1
	ExecutionManifest *JenkinsExecutionManifestV1
	lines             [][]byte
}

// ReadCleaned reads a package written by Clean. A record keeps its line number there, so the number of
// a record in the journal finds it.
func ReadCleaned(r io.Reader, limits Limits) (*Package, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("envelope: open cleaned package: %w", err)
	}
	tr := tar.NewReader(gz)
	p := &Package{}
	budget := limits.MaxUnpackedBytes
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("envelope: read cleaned package: %w", err)
		}
		if header.Name != ManifestName && header.Name != RecordsName && header.Name != ScanManifestName && header.Name != ExecutionManifestName {
			continue
		}
		if header.Size > budget {
			return nil, fmt.Errorf("envelope: %s is larger than the limit", header.Name)
		}
		budget -= header.Size
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("envelope: read %s: %w", header.Name, err)
		}
		if header.Name == ManifestName {
			p.Manifest = &Manifest{}
			if err := json.Unmarshal(data, p.Manifest); err != nil {
				return nil, fmt.Errorf("envelope: decode manifest: %w", err)
			}
			continue
		}
		if header.Name == ScanManifestName {
			p.ScanManifest = &ScanManifestV1{}
			if err := json.Unmarshal(data, p.ScanManifest); err != nil {
				return nil, fmt.Errorf("envelope: decode scan manifest: %w", err)
			}
			continue
		}
		if header.Name == ExecutionManifestName {
			p.ExecutionManifest = &JenkinsExecutionManifestV1{}
			if err := json.Unmarshal(data, p.ExecutionManifest); err != nil {
				return nil, fmt.Errorf("envelope: decode execution manifest: %w", err)
			}
			continue
		}
		p.lines = bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	}
	if p.Manifest == nil {
		return nil, errNoManifest
	}
	return p, nil
}

// Record decodes the record of a number; false when the package has none there.
func (p *Package) Record(no int) (*Record, bool, error) {
	if no < 1 || no > len(p.lines) || len(p.lines[no-1]) == 0 {
		return nil, false, nil
	}
	var rec Record
	if err := json.Unmarshal(p.lines[no-1], &rec); err != nil {
		return nil, false, fmt.Errorf("envelope: decode record %d: %w", no, err)
	}
	return &rec, true, nil
}

// Files streams the named files of a cleaned package in the order they are stored: visit reads one file before the
// next is opened, so a report is never held in memory whole. A name the package lacks is an error.
func Files(r io.Reader, names []string, visit func(name string, content io.Reader) error) error {
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("envelope: open cleaned package: %w", err)
	}
	tr := tar.NewReader(gz)
	for len(wanted) > 0 {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("envelope: read cleaned package: %w", err)
		}
		if !wanted[header.Name] {
			continue
		}
		delete(wanted, header.Name)
		if err := visit(header.Name, tr); err != nil {
			return err
		}
	}
	for name := range wanted {
		return fmt.Errorf("envelope: cleaned package has no file %s", name)
	}
	return nil
}
