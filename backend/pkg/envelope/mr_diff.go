package envelope

import (
	"encoding/json"
	"path"
	"strings"
)

// ValidMRDiffPath accepts a repository-relative Git path as it appears in the diff.
func ValidMRDiffPath(value string) bool {
	return value != "" && value != "." && value != ".." && !path.IsAbs(value) &&
		!strings.HasPrefix(value, "../") && !strings.ContainsRune(value, 0) && path.Clean(value) == value
}

func validateMRDiff(record, payload map[string]any) string {
	if record["native_ns"] != payload["mr_native_id"] {
		return "native_ns: expected the merge request ID of the diff"
	}
	seen := map[[2]string]bool{}
	files, _ := payload["files"].([]any)
	for _, file := range files {
		f, _ := file.(map[string]any)
		oldPath, newPath := text(f, "old_path"), text(f, "new_path")
		if !ValidMRDiffPath(oldPath) || !ValidMRDiffPath(newPath) {
			return "payload.files: expected repository-relative paths"
		}
		key := [2]string{oldPath, newPath}
		if seen[key] {
			return "payload.files: repeated file"
		}
		seen[key] = true
		if !validDiffFile(f, payload["completeness"] == "complete") {
			return "payload.files: inconsistent flags, line counts or head ranges"
		}
	}
	return ""
}

func validDiffFile(f map[string]any, complete bool) bool {
	added, addedKnown := f["added_lines"].(json.Number)
	deleted, deletedKnown := f["deleted_lines"].(json.Number)
	ranges, _ := f["head_ranges"].([]any)
	newFile, deletedFile, renamed := f["new_file"] == true, f["deleted_file"] == true, f["renamed_file"] == true
	if newFile && deletedFile || renamed && (newFile || deletedFile || f["old_path"] == f["new_path"]) {
		return false
	}
	if !addedKnown || !deletedKnown {
		return !complete && !addedKnown && !deletedKnown && len(ranges) == 0
	}
	addedLines, _ := added.Int64()
	deletedLines, _ := deleted.Int64()
	if (deletedFile || f["binary"] == true) && addedLines != 0 || (newFile || f["binary"] == true) && deletedLines != 0 {
		return false
	}
	var end, covered int64
	for _, each := range ranges {
		r, _ := each.(map[string]any)
		start, _ := r["from"].(json.Number)
		stop, _ := r["to"].(json.Number)
		from, fromErr := start.Int64()
		to, toErr := stop.Int64()
		if fromErr != nil || toErr != nil || from <= end || to < from {
			return false
		}
		end = to
		covered += to - from + 1
	}
	return covered == addedLines
}
