package ci

import (
	"bytes"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// _anchorDrop are the characters a GitHub heading anchor drops: all but letters, digits, spaces, "_" and "-".
var _anchorDrop = regexp.MustCompile(`[^\p{L}\p{N} _-]`)

// markdownDoc is what the docs profile reads from a Markdown file: the owner from front matter, the title,
// the heading anchors and the links with their lines.
type markdownDoc struct {
	owner   string
	title   string
	anchors []string
	links   []markdownLink
}

type markdownLink struct {
	target string
	line   int
}

// parseMarkdown parses the file with goldmark. The front matter block, "---" lines around "key: value"
// pairs at the top, gives the owner under ownerKey; its lines are blanked so that lines keep their numbers.
func parseMarkdown(src []byte, ownerKey string) markdownDoc {
	var doc markdownDoc
	src, doc.owner = frontMatter(src, ownerKey)
	starts := lineStarts(src)
	root := goldmark.New().Parser().Parse(text.NewReader(src))
	seen := map[string]int{}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			heading := strings.TrimSpace(inlineText(node, src))
			if doc.title == "" && node.Level == 1 {
				doc.title = heading
			}
			doc.anchors = append(doc.anchors, githubAnchor(heading, seen))
		case *ast.Link:
			doc.links = append(doc.links, markdownLink{target: string(node.Destination), line: nodeLine(node, starts)})
		case *ast.Image:
			doc.links = append(doc.links, markdownLink{target: string(node.Destination), line: nodeLine(node, starts)})
		}
		return ast.WalkContinue, nil
	})
	return doc
}

// frontMatter blanks the front matter of the file and returns the value of the owner key in it.
func frontMatter(src []byte, ownerKey string) (blanked []byte, owner string) {
	if !bytes.HasPrefix(src, []byte("---\n")) {
		return src, ""
	}
	end := bytes.Index(src[4:], []byte("\n---\n"))
	if end < 0 {
		return src, ""
	}
	header := src[4 : 4+end]
	for _, line := range strings.Split(string(header), "\n") {
		if value, ok := strings.CutPrefix(line, ownerKey+":"); ok && owner == "" {
			owner = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	blanked = bytes.Clone(src)
	for i := range 4 + end + 4 {
		if blanked[i] != '\n' {
			blanked[i] = ' '
		}
	}
	return blanked, owner
}

// githubAnchor is the GitHub and GitLab anchor of a heading: lower case, punctuation dropped, each space a
// hyphen; a repeated anchor gets "-1", "-2" and so on.
func githubAnchor(heading string, seen map[string]int) string {
	anchor := strings.ReplaceAll(_anchorDrop.ReplaceAllString(strings.ToLower(heading), ""), " ", "-")
	n := seen[anchor]
	seen[anchor] = n + 1
	if n > 0 {
		return anchor + "-" + strconv.Itoa(n)
	}
	return anchor
}

// inlineText joins the text of the inline children of a node: text, code spans and the text of links.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// nodeLine is the line of an inline node: the first text segment inside it, or the first line of its block.
func nodeLine(n ast.Node, starts []int) int {
	offset := -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering && offset < 0 {
			offset = t.Segment.Start
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	for p := n.Parent(); offset < 0 && p != nil; p = p.Parent() {
		if p.Type() == ast.TypeBlock && p.Lines().Len() > 0 {
			offset = p.Lines().At(0).Start
		}
	}
	if offset < 0 {
		return 1
	}
	line, _ := slices.BinarySearch(starts, offset+1)
	return line
}

func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}
