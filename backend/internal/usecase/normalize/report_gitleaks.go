package normalize

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
)

func parseGitleaks(r io.Reader, in parseInput) (*parsedReport, error) {
	dec := json.NewDecoder(r)
	out := &parsedReport{formatVersion: "1"}
	err := array(dec, func() error {
		var row struct {
			RuleID, File, Fingerprint string
			Commit                    string
			StartLine, EndLine        int
			StartColumn, EndColumn    int
		}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		f, err := newFinding(in.kind, row.RuleID, "")
		if err != nil {
			return err
		}
		f.Path = securityPath(row.File)
		f.LineStart, f.LineEnd = trivyLines(row.StartLine, row.EndLine)
		if hash, err := hex.DecodeString(row.Fingerprint); err == nil && len(hash) == 32 {
			// The builtin detector already deduplicates this identity over the checked range.
			f.Fingerprint = fingerprint("gitleaks/json/1", f.RuleID, f.Path, string(hash))
		} else {
			f.Fingerprint = fingerprint("gitleaks/json/1", f.RuleID, f.Path, lineText(f.LineStart), lineText(f.LineEnd),
				strconv.Itoa(row.StartColumn), strconv.Itoa(row.EndColumn), commitHash(row.Commit))
		}
		out.findings = append(out.findings, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, end(dec)
}

func commitHash(s string) string {
	if len(s) == 40 || len(s) == 64 {
		if _, err := hex.DecodeString(s); err == nil {
			return s
		}
	}
	return ""
}
