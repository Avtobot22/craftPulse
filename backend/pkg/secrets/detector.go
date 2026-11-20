// Package secrets shares the pinned detector and redaction policy between the CI step and intake.
package secrets

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/logging"
)

const (
	ProfileID       = "gitleaks-default"
	GitleaksVersion = "8.30.1"
)

var (
	//go:embed profiles/gitleaks-default/rules.toml
	_rules string
	//go:embed profiles/gitleaks-default/profile.json
	_profile []byte
)

func init() { logging.Logger = zerolog.Nop() } // Upstream trace messages contain secret matches.

type Finding struct {
	RuleID      string `json:"RuleID"`    //nolint:tagliatelle // Gitleaks format.
	File        string `json:"File"`      //nolint:tagliatelle // Gitleaks format.
	StartLine   int    `json:"StartLine"` //nolint:tagliatelle // Gitleaks format.
	EndLine     int    `json:"-"`
	Commit      string `json:"Commit"`      //nolint:tagliatelle // Gitleaks format.
	Fingerprint string `json:"Fingerprint"` //nolint:tagliatelle // Gitleaks format.
}
type Detector struct {
	detector *detect.Detector
	Scrubber *Scrubber
}

func New(ctx context.Context) (*Detector, error) {
	var policy struct {
		MinFragmentLength int `json:"min_fragment_length"`
	}
	if json.Unmarshal(_profile, &policy) != nil || policy.MinFragmentLength != 8 {
		return nil, errors.New("invalid secrets policy")
	}
	sum := sha256.Sum256([]byte("craftpulse fixture marker, not a credential"))
	marker := base64.RawURLEncoding.EncodeToString(sum[:])[:32]
	rules := strings.ReplaceAll(_rules, "{{CRAFTPULSE_DEMO_MARKER}}", regexp.QuoteMeta(marker))
	v := viper.New()
	v.SetConfigType("toml")
	if v.ReadConfig(strings.NewReader(config.DefaultConfig+"\n"+rules)) != nil {
		return nil, errors.New("invalid secrets rules")
	}
	var vc config.ViperConfig
	if v.Unmarshal(&vc) != nil {
		return nil, errors.New("invalid secrets rules")
	}
	cfg, err := vc.Translate()
	if err != nil {
		return nil, errors.New("invalid secrets rules")
	}
	d := detect.NewDetectorContext(ctx, cfg)
	d.IgnoreGitleaksAllow = true
	return &Detector{detector: d, Scrubber: NewScrubber(policy.MinFragmentLength)}, nil
}

func RulesHash() string {
	sum := sha256.Sum256([]byte(GitleaksVersion + "\n" + _rules + string(_profile)))
	return hex.EncodeToString(sum[:])
}

func (d *Detector) Scan(ctx context.Context, content []byte, path, commit string) ([]Finding, error) {
	found := d.detector.DetectContext(ctx, detect.Fragment{Raw: string(content), FilePath: path, CommitSHA: commit, StartLine: 1}) //nolint:staticcheck // The pinned v8 API requires detect.Fragment.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]Finding, 0, len(found))
	for i := range found {
		f := &found[i]
		d.Scrubber.Add(f.Secret)
		secretHash := sha256.Sum256([]byte(f.Secret))
		identity, _ := json.Marshal([]string{f.RuleID, f.File, hex.EncodeToString(secretHash[:])})
		fingerprint := sha256.Sum256(identity)
		out = append(out, Finding{RuleID: f.RuleID, File: f.File, StartLine: f.StartLine, EndLine: f.EndLine, Commit: f.Commit, Fingerprint: hex.EncodeToString(fingerprint[:])})
	}
	return out, nil
}

// Discover also checks decoded strings so JSON/XML escaping cannot bypass detection.
func (d *Detector) Discover(ctx context.Context, name string, data []byte) error {
	scan := func(content []byte) error {
		for _, candidate := range []string{name, "ingest.text"} {
			if _, err := d.Scan(ctx, content, candidate, ""); err != nil {
				return err
			}
		}
		return nil
	}
	if err := scan(data); err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if json.Valid(data) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return errors.New("cannot decode text for secret detection")
		}
		canonical, _ := json.Marshal(value)
		return scan(canonical)
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "<") {
		decoder := xml.NewDecoder(bytes.NewReader(data))
		var text bytes.Buffer
		flush := func() error {
			if text.Len() == 0 {
				return nil
			}
			err := scan(text.Bytes())
			text.Reset()
			return err
		}
		for {
			token, err := decoder.RawToken()
			if err != nil {
				return flush()
			} // Raw text was scanned; markup decoding is best effort.
			switch value := token.(type) {
			case xml.CharData:
				text.Write(value)
			case xml.Comment:
			default:
				if err := flush(); err != nil {
					return err
				}
				if start, ok := token.(xml.StartElement); ok {
					for _, attr := range start.Attr {
						if err := scan([]byte(attr.Name.Local + ` = "` + attr.Value + `"`)); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
