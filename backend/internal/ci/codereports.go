package ci

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
)

const _sarifVersion = "2.1.0"

type sarifParts struct {
	Version string            `json:"version"`
	Schema  string            `json:"$schema,omitempty"` //nolint:tagliatelle // SARIF wire format.
	Runs    []json.RawMessage `json:"runs"`
}

// mergeSARIF joins the runs of SARIF 2.1.0 reports into one report.
func mergeSARIF(parts []string, dst string) error {
	merged := sarifParts{Version: _sarifVersion, Runs: []json.RawMessage{}}
	for _, name := range parts {
		data, err := os.ReadFile(name) //nolint:gosec // the step names its own report parts
		if err != nil {
			return fmt.Errorf("read linter report: %w", err)
		}
		var part sarifParts
		if err := json.Unmarshal(data, &part); err != nil {
			return fmt.Errorf("linter report is not SARIF JSON: %w", err)
		}
		if part.Version != _sarifVersion {
			return fmt.Errorf("linter report has SARIF version %q", part.Version)
		}
		merged.Schema = part.Schema
		merged.Runs = append(merged.Runs, part.Runs...)
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encode linter report: %w", err)
	}
	return os.WriteFile(dst, data, 0o600)
}

// stripClones copies the jscpd JSON report without the code of the copies: the report keeps files,
// lines and tokens of every clone.
func stripClones(src, dst string) error {
	data, err := os.ReadFile(src) //nolint:gosec // the step names its own report
	if err != nil {
		return fmt.Errorf("read clone report: %w", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("clone report is not JSON: %w", err)
	}
	duplicates, ok := report["duplicates"].([]any)
	if !ok {
		return errors.New("clone report has no duplicates list")
	}
	for _, d := range duplicates {
		if clone, ok := d.(map[string]any); ok {
			delete(clone, "fragment")
		}
	}
	if data, err = json.Marshal(report); err != nil {
		return fmt.Errorf("encode clone report: %w", err)
	}
	return os.WriteFile(dst, data, 0o600)
}

// functionFacts reads a Lizard CSV report: one fact per function with its file, first line, name,
// cyclomatic complexity and NLOC.
func functionFacts(r io.Reader) ([]string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 11
	var out []string
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("function report is not Lizard CSV: %w", err)
		}
		nloc, ccn, file, name, start := rec[0], rec[1], rec[6], rec[7], rec[9]
		out = append(out, functionFact(path.Clean(file), start, name, ccn, nloc))
	}
}

func functionFact(file, line, name, ccn, nloc string) string {
	return "function " + file + ":" + line + " " + name + " ccn=" + ccn + " nloc=" + nloc
}

type jscpdReport struct {
	Duplicates []struct {
		FirstFile  jscpdLocation `json:"firstFile"`  //nolint:tagliatelle // jscpd wire format.
		SecondFile jscpdLocation `json:"secondFile"` //nolint:tagliatelle // jscpd wire format.
	} `json:"duplicates"`
}

type jscpdLocation struct {
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func (l jscpdLocation) String() string {
	return path.Clean(l.Name) + ":" + strconv.Itoa(l.Start) + "-" + strconv.Itoa(l.End)
}

// cloneFacts reads a jscpd JSON report: one fact per pair of copies, the two places in path order.
func cloneFacts(r io.Reader) ([]string, error) {
	var report jscpdReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("clone report is not jscpd JSON: %w", err)
	}
	out := make([]string, 0, len(report.Duplicates))
	for _, d := range report.Duplicates {
		out = append(out, cloneFact(d.FirstFile.String(), d.SecondFile.String()))
	}
	return out, nil
}

func cloneFact(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return "clone " + a + " " + b
}

// scopeFacts reads the scope of a fixture run: the test files, the excluded paths and the skipped files with
// their reason.
func scopeFacts(r io.Reader) ([]string, error) {
	var s FixtureScope
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, fmt.Errorf("scope report is not JSON: %w", err)
	}
	out := make([]string, 0, len(s.Tests)+len(s.Excluded)+len(s.Skipped))
	for _, t := range s.Tests {
		out = append(out, "test "+t)
	}
	for _, e := range s.Excluded {
		out = append(out, "excluded "+e.Path+" "+string(e.Reason))
	}
	for _, e := range s.Skipped {
		out = append(out, "skipped "+e.Path+" "+string(e.Reason))
	}
	return out, nil
}
