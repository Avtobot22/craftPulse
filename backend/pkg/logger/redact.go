package logger

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"slices"
)

var _mask = []byte("[REDACTED]")

type redactor struct {
	w        io.Writer
	patterns [][]byte
}

func newRedactor(w io.Writer, secrets []string) *redactor {
	var patterns [][]byte
	for _, s := range secrets {
		if s == "" {
			continue
		}
		patterns = append(patterns, []byte(s))
		quoted, _ := json.Marshal(s)
		if escaped := quoted[1 : len(quoted)-1]; !bytes.Equal(escaped, []byte(s)) {
			patterns = append(patterns, escaped)
		}
	}
	// A secret that contains another one must be masked whole.
	slices.SortFunc(patterns, func(a, b []byte) int { return cmp.Compare(len(b), len(a)) })
	return &redactor{w: w, patterns: patterns}
}

func (r *redactor) Write(p []byte) (int, error) {
	out := p
	for _, pat := range r.patterns {
		out = bytes.ReplaceAll(out, pat, _mask)
	}
	if _, err := r.w.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}
