package ingest

import (
	"bytes"
	"context"
	"io"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
	"github.com/Avtobot22/craftPulse/backend/pkg/secrets"
)

type packageRedactor struct {
	ctx      context.Context
	detector *secrets.Detector
}

func (r *packageRedactor) Discover(name string, data []byte) error {
	if name == envelope.RecordsName {
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			if len(bytes.TrimSpace(line)) > 0 {
				if err := r.detector.Discover(r.ctx, name, line); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return r.detector.Discover(r.ctx, name, data)
}

func (r *packageRedactor) Redact(name string, data []byte) (sanitized []byte, replacements int64, err error) {
	before := r.detector.Scrubber.Replacements()
	if name != envelope.RecordsName {
		safe, err := r.detector.Scrubber.Redact(data)
		return safe, r.detector.Scrubber.Replacements() - before, err
	}
	var out bytes.Buffer
	lines := bytes.Split(data, []byte{'\n'})
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		if len(bytes.TrimSpace(line)) == 0 {
			out.Write(line)
			continue
		}
		safe, err := r.detector.Scrubber.Redact(line)
		if err != nil {
			return nil, 0, err
		}
		out.Write(safe)
	}
	return out.Bytes(), r.detector.Scrubber.Replacements() - before, nil
}

// clean checks and redacts a package; a page of sync may carry report files of the source without a scan manifest.
func (u *UseCase) clean(ctx context.Context, channel entity.ReceiptChannel, r io.Reader, w io.Writer) (*envelope.Result, error) {
	detector, err := secrets.New(ctx)
	if err != nil {
		return nil, err
	}
	redactor := &packageRedactor{ctx: ctx, detector: detector}
	if channel == entity.ChannelSync {
		return envelope.CleanPageWithRedactor(r, u.d.Limits, w, redactor)
	}
	return envelope.CleanWithRedactor(r, u.d.Limits, w, redactor)
}

func (r *packageRedactor) VerifyPackage(data []byte) error {
	return r.detector.Scrubber.VerifyPackage(data)
}
