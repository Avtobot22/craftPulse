package normalize

import (
	"net/url"
	"path"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const _docsSARIFParserVersion = "sarif-2.1.0-docs/2"

func docsLinkEvidence(tool string, f *entity.FindingOccurrence, res *sarifResult) *envelope.DocsLinkTarget {
	if f.Kind != entity.FindingDocs || (f.RuleID != entity.DocsRuleMissingFile && f.RuleID != entity.DocsRuleMissingAnchor) {
		return nil
	}
	target := res.Properties.text("target_path")
	anchor := res.Properties.text("anchor")
	if target == "" && tool == "craftpulse-docs" {
		target, anchor = legacyDocsLink(f.Path, f.RuleID, res.Message.Text)
	}
	if target == "" || strings.ContainsAny(target, "?#\\\x00\r\n") || strings.HasPrefix(target, "/") ||
		path.Clean(target) != target || len(target) > _maxPath || len(anchor) > _maxPath ||
		strings.ContainsAny(anchor, "\x00\r\n") {
		return nil
	}
	return &envelope.DocsLinkTarget{TargetPath: target, Anchor: anchor}
}

// Version 1 of the bundled producer put the target in its message. Keep only the local coordinates.
func legacyDocsLink(from, rule, message string) (targetPath, anchor string) {
	target, ok := strings.CutPrefix(message, "link target ")
	if !ok {
		return "", ""
	}
	if rule == entity.DocsRuleMissingAnchor {
		target, _, ok = strings.Cut(target, ": no heading of ")
	} else {
		target, ok = strings.CutSuffix(target, " does not exist")
	}
	if !ok {
		return "", ""
	}
	file, anchor, _ := strings.Cut(target, "#")
	file, _, _ = strings.Cut(file, "?")
	if u, err := url.PathUnescape(file); err == nil {
		file = u
	}
	if u, err := url.PathUnescape(anchor); err == nil {
		anchor = u
	}
	switch {
	case file == "":
		return from, anchor
	case strings.HasPrefix(file, "/"):
		return path.Clean(strings.TrimLeft(file, "/")), anchor
	default:
		return path.Join(path.Dir(from), file), anchor
	}
}
