package dataset

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

type docSpec struct {
	path     string
	title    string
	owner    string
	sections []string
	links    []string
	// index links every other document of the repository.
	index bool
}

type specOperation struct {
	method, path, id, summary, response string
}

type openAPISpec struct {
	path, title string
	valid       bool
	operations  []specOperation
}

var (
	_linkPattern    = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	_headingPattern = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)
	_slugDrop       = regexp.MustCompile(`[^\p{L}\p{N} _-]`)
)

func renderDocs(docs []docSpec, specs []openAPISpec) map[string][]byte {
	all := make([]string, 0, len(docs)+len(specs))
	for _, doc := range docs {
		all = append(all, doc.path)
	}
	for _, spec := range specs {
		all = append(all, spec.path)
	}
	files := make(map[string][]byte, len(all))
	for _, doc := range docs {
		links := doc.links
		if doc.index {
			links = nil
			for _, other := range all {
				if other != doc.path {
					links = append(links, relativeTo(doc.path, other))
				}
			}
		}
		files[doc.path] = []byte(renderMarkdown(doc, links))
	}
	for _, spec := range specs {
		files[spec.path] = []byte(renderOpenAPI(spec))
	}
	return files
}

func renderMarkdown(doc docSpec, links []string) string {
	var b strings.Builder
	if doc.owner != "" {
		fmt.Fprintf(&b, "---\nowner: %s\n---\n\n", doc.owner)
	}
	fmt.Fprintf(&b, "# %s\n\n%s describes this part of the service for the team.\n", doc.title, doc.title)
	for _, section := range doc.sections {
		fmt.Fprintf(&b, "\n## %s\n\nNotes on %s.\n", section, strings.ToLower(section))
	}
	if len(links) > 0 {
		b.WriteString("\n## See also\n\n")
		for _, link := range links {
			label, _, _ := strings.Cut(link, "#")
			fmt.Fprintf(&b, "- [%s](%s)\n", path.Base(label), link)
		}
	}
	return b.String()
}

// renderOpenAPI writes an OpenAPI 3.0 document; an invalid one omits the required response description.
func renderOpenAPI(spec openAPISpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "openapi: 3.0.3\ninfo:\n  title: %s\n  version: 1.0.0\npaths:\n", spec.title)
	byPath := make(map[string][]specOperation)
	var paths []string
	for _, op := range spec.operations {
		if byPath[op.path] == nil {
			paths = append(paths, op.path)
		}
		byPath[op.path] = append(byPath[op.path], op)
	}
	for _, p := range paths {
		fmt.Fprintf(&b, "  %s:\n", p)
		for _, op := range byPath[p] {
			fmt.Fprintf(&b, "    %s:\n      operationId: %s\n      summary: %s\n", op.method, op.id, op.summary)
			if name, ok := pathParameter(op.path); ok {
				fmt.Fprintf(&b, "      parameters:\n        - name: %s\n          in: path\n          required: true\n          schema:\n            type: string\n", name)
			}
			b.WriteString("      responses:\n        \"200\":\n")
			if spec.valid {
				fmt.Fprintf(&b, "          description: %s\n", op.response)
			} else {
				b.WriteString("          content:\n            application/json:\n              schema:\n                type: object\n")
			}
		}
	}
	return b.String()
}

func pathParameter(p string) (string, bool) {
	_, rest, ok := strings.Cut(p, "{")
	if !ok {
		return "", false
	}
	name, _, ok := strings.Cut(rest, "}")
	return name, ok
}

func relativeTo(from, to string) string {
	fromDir := strings.Split(path.Dir(from), "/")
	target := strings.Split(to, "/")
	common := 0
	for common < len(fromDir) && common < len(target)-1 && fromDir[common] == target[common] {
		common++
	}
	parts := slices.Repeat([]string{".."}, len(fromDir)-common)
	return strings.Join(append(parts, target[common:]...), "/")
}

// scanDocuments reads the documents under the roots of a tree: owners from front matter, anchors from
// headings and local links resolved against the tree. Validity of OpenAPI documents comes from specValid.
func scanDocuments(repoID string, roots []string, files map[string][]byte, specValid map[string]bool) []GitDocument {
	var paths []string
	for p := range files {
		if underRoots(p, roots) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	anchors := make(map[string][]string)
	for _, p := range paths {
		if strings.HasSuffix(p, ".md") {
			anchors[p] = headingAnchors(string(files[p]))
		}
	}
	docs := make([]GitDocument, 0, len(paths))
	for _, p := range paths {
		doc := GitDocument{ID: repoID + ":" + p, Path: p, Format: FormatMarkdown, Anchors: []string{}, Links: []DocLink{}}
		if !strings.HasSuffix(p, ".md") {
			doc.Format = FormatOpenAPI
			valid := specValid[p]
			doc.SpecValid = &valid
			docs = append(docs, doc)
			continue
		}
		content := string(files[p])
		doc.Owner = frontMatterOwner(content)
		doc.Anchors = anchors[p]
		for i, line := range strings.Split(content, "\n") {
			for _, match := range _linkPattern.FindAllStringSubmatch(line, -1) {
				if link, ok := resolveLink(p, match[1], files, anchors); ok {
					link.Line = i + 1
					doc.Links = append(doc.Links, link)
				}
			}
		}
		docs = append(docs, doc)
	}
	return docs
}

func underRoots(p string, roots []string) bool {
	for _, root := range roots {
		if strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

func frontMatterOwner(content string) string {
	header, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return ""
	}
	header, _, ok = strings.Cut(header, "\n---\n")
	if !ok {
		return ""
	}
	for _, line := range strings.Split(header, "\n") {
		if owner, ok := strings.CutPrefix(line, "owner:"); ok {
			return strings.TrimSpace(owner)
		}
	}
	return ""
}

func headingAnchors(content string) []string {
	anchors := []string{}
	for _, line := range strings.Split(content, "\n") {
		if match := _headingPattern.FindStringSubmatch(line); match != nil {
			anchors = append(anchors, slug(match[1]))
		}
	}
	return anchors
}

// slug follows the GitLab and GitHub heading anchors: lower case, punctuation dropped, spaces to hyphens.
func slug(heading string) string {
	return strings.ReplaceAll(_slugDrop.ReplaceAllString(strings.ToLower(heading), ""), " ", "-")
}

func resolveLink(from, target string, files map[string][]byte, anchors map[string][]string) (DocLink, bool) {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return DocLink{}, false
	}
	file, anchor, _ := strings.Cut(target, "#")
	resolved := from
	if file != "" {
		resolved = path.Join(path.Dir(from), file)
	}
	link := DocLink{Target: target, Path: resolved, Anchor: anchor}
	if _, exists := files[resolved]; !exists || strings.HasPrefix(resolved, "../") {
		link.Broken = BrokenFile
		return link, true
	}
	if anchor != "" && !slices.Contains(anchors[resolved], anchor) {
		link.Broken = BrokenAnchor
	}
	return link, true
}
