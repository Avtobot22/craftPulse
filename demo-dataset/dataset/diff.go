package dataset

import (
	"fmt"
	"sort"
	"strings"
)

const _diffContext = 3

// unifiedDiff renders a patch that git apply accepts. Every changed file gets one hunk from the first to
// the last changed line, so separate edits of one file share a hunk with the lines between them as context.
func unifiedDiff(before, after map[string][]byte) string {
	paths := make([]string, 0, len(after))
	for path, content := range after {
		if old, exists := before[path]; !exists || string(old) != string(content) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var out strings.Builder
	for _, path := range paths {
		old, exists := before[path]
		fmt.Fprintf(&out, "diff --git a/%s b/%s\n", path, path)
		if !exists {
			fmt.Fprintf(&out, "new file mode 100644\nindex %s..%s\n--- /dev/null\n", strings.Repeat("0", 40), blobID(after[path]))
		} else {
			fmt.Fprintf(&out, "index %s..%s 100644\n--- a/%s\n", blobID(old), blobID(after[path]), path)
		}
		fmt.Fprintf(&out, "+++ b/%s\n", path)
		writeHunk(&out, lines(old), lines(after[path]))
	}
	return out.String()
}

func writeHunk(out *strings.Builder, old, updated []string) {
	prefix := 0
	for prefix < len(old) && prefix < len(updated) && old[prefix] == updated[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(updated)-prefix && old[len(old)-1-suffix] == updated[len(updated)-1-suffix] {
		suffix++
	}
	start := max(0, prefix-_diffContext)
	tail := min(suffix, _diffContext)
	oldPart := old[start : len(old)-suffix+tail]
	newPart := updated[start : len(updated)-suffix+tail]
	fmt.Fprintf(out, "@@ -%s +%s @@\n", hunkRange(start, len(oldPart)), hunkRange(start, len(newPart)))
	for i := start; i < prefix; i++ {
		fmt.Fprintf(out, " %s\n", old[i])
	}
	for _, line := range old[prefix : len(old)-suffix] {
		fmt.Fprintf(out, "-%s\n", line)
	}
	for _, line := range updated[prefix : len(updated)-suffix] {
		fmt.Fprintf(out, "+%s\n", line)
	}
	for _, line := range old[len(old)-suffix : len(old)-suffix+tail] {
		fmt.Fprintf(out, " %s\n", line)
	}
}

// hunkRange numbers lines from one; an empty range points at the line before it.
func hunkRange(start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	return fmt.Sprintf("%d,%d", start+1, count)
}

// lines splits text that ends with a newline; fixture files always do.
func lines(content []byte) []string {
	text := strings.TrimSuffix(string(content), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
