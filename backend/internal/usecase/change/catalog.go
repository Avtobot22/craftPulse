package change

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

var (
	_digest    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	_profileID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	_language  = regexp.MustCompile(`^[a-z0-9+#-]{1,32}$`)
	_kinds     = []string{
		entity.AnalysisCode, entity.AnalysisSAST, entity.AnalysisSCA, entity.AnalysisSecrets, entity.AnalysisIaC, entity.AnalysisImage,
		entity.AnalysisJUnit, entity.AnalysisCoverage, entity.AnalysisDocs, entity.AnalysisOpenAPI, entity.AnalysisSonar,
	}
)

// catalogFile is the catalog of profiles of a release as the file keeps it. The step image runs the upload of
// ready reports; a builtin profile runs from its image by digest with its script, an import profile has neither.
type catalogFile struct {
	Version int `yaml:"version"`
	Step    struct {
		Image  string `yaml:"image"`
		Digest string `yaml:"digest"`
	} `yaml:"step"`
	Profiles []struct {
		ID            string            `yaml:"id"`
		Mode          string            `yaml:"mode"`
		Languages     []string          `yaml:"languages"`
		AnyLanguage   bool              `yaml:"any_language"`
		AnalysisKinds []string          `yaml:"analysis_kinds"`
		Image         string            `yaml:"image"`
		Digest        string            `yaml:"digest"`
		ToolVersions  map[string]string `yaml:"tool_versions"`
		RulesHash     string            `yaml:"rules_hash"`
		Script        []string          `yaml:"script"`
		ReportFormat  string            `yaml:"report_format"`
	} `yaml:"profiles"`
}

// ReadCatalog reads the catalog of profiles of a release from its file (FR-011 A1).
func ReadCatalog(path string) (entity.CheckCatalog, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the configuration of the installation
	if err != nil {
		return entity.CheckCatalog{}, fmt.Errorf("change: read the catalog of check profiles: %w", err)
	}
	return ParseCatalog(raw)
}

// ParseCatalog checks a catalog: version 1, the step image by digest, unique profiles with known analysis
// kinds, an image by digest and a script for a builtin profile and none for an import one.
func ParseCatalog(raw []byte) (entity.CheckCatalog, error) {
	var f catalogFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return entity.CheckCatalog{}, fmt.Errorf("change: parse the catalog of check profiles: %w", err)
	}
	var errs []error
	if f.Version != 1 {
		errs = append(errs, fmt.Errorf("version %d is not supported", f.Version))
	}
	if f.Step.Image == "" || !_digest.MatchString(f.Step.Digest) {
		errs = append(errs, errors.New("step: an image and a sha256 digest are required"))
	}
	out := entity.CheckCatalog{StepImage: f.Step.Image, StepDigest: f.Step.Digest}
	seen := map[string]bool{}
	for i := range f.Profiles {
		p := &f.Profiles[i]
		at := fmt.Sprintf("profiles[%d]", i)
		if !_profileID.MatchString(p.ID) || seen[p.ID] {
			errs = append(errs, fmt.Errorf("%s: id %q is invalid or repeated", at, p.ID))
		}
		seen[p.ID] = true
		if len(p.AnalysisKinds) == 0 {
			errs = append(errs, fmt.Errorf("%s: analysis_kinds are required", at))
		}
		for _, k := range p.AnalysisKinds {
			if !slices.Contains(_kinds, k) {
				errs = append(errs, fmt.Errorf("%s: analysis kind %q is unknown", at, k))
			}
		}
		for _, l := range p.Languages {
			if !_language.MatchString(l) {
				errs = append(errs, fmt.Errorf("%s: language %q is invalid", at, l))
			}
		}
		if p.AnyLanguage && len(p.Languages) > 0 {
			errs = append(errs, fmt.Errorf("%s: any_language excludes languages", at))
		}
		switch p.Mode {
		case entity.CheckModeBuiltin:
			if p.Image == "" || !_digest.MatchString(p.Digest) || len(p.Script) == 0 {
				errs = append(errs, fmt.Errorf("%s: a builtin profile needs an image, a sha256 digest and a script", at))
			}
		case entity.CheckModeImport:
			if p.Image != "" || p.Digest != "" || len(p.Script) > 0 {
				errs = append(errs, fmt.Errorf("%s: an import profile has no image and no script", at))
			}
		default:
			errs = append(errs, fmt.Errorf("%s: mode %q is not import or builtin", at, p.Mode))
		}
		var rules []byte
		if p.RulesHash != "" {
			h, err := hex.DecodeString(p.RulesHash)
			if err != nil || len(h) != 32 {
				errs = append(errs, fmt.Errorf("%s: rules_hash is not a hex SHA-256", at))
			}
			rules = h
		}
		tools := p.ToolVersions
		if tools == nil {
			tools = map[string]string{}
		}
		out.Profiles = append(out.Profiles, entity.CheckProfile{
			ID: p.ID, Mode: p.Mode, Languages: p.Languages, AnyLanguage: p.AnyLanguage, AnalysisKinds: p.AnalysisKinds, Image: p.Image,
			Digest: p.Digest, ToolVersions: tools, RulesHash: rules, Script: p.Script, ReportFormat: p.ReportFormat,
		})
	}
	if err := errors.Join(errs...); err != nil {
		return entity.CheckCatalog{}, fmt.Errorf("change: invalid catalog of check profiles: %w", err)
	}
	return out, nil
}
