package dataset

import (
	"strings"
	"testing"
)

// TestDocTexts_KeepLinksAndAnchorsToTheTables: a text adds no link and no heading, and every text belongs to a document
// or a section of the tables.
func TestDocTexts_KeepLinksAndAnchorsToTheTables(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	for _, spec := range repoSpecs() {
		for _, doc := range spec.docs {
			used[spec.key+" "+doc.path] = true
			for _, section := range doc.sections {
				used[spec.key+" "+doc.path+"#"+section] = true
			}
		}
	}
	for key, text := range _docTexts {
		if !used[key] {
			t.Errorf("text %q belongs to no document", key)
		}
		if _linkPattern.MatchString(text) || strings.Contains(text, "\n") || strings.HasPrefix(text, "#") {
			t.Errorf("text %q adds a link, a line or a heading", key)
		}
	}
}
