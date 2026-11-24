package gitlab

import (
	"cmp"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

const (
	_maxDiffFiles        = 1000
	_maxHeadRanges       = 10000
	_maxDiffPayloadBytes = 2 << 20
)

type diffDetails struct {
	diffVersion
	State    string            `json:"state"`
	RealSize string            `json:"real_size"`
	Files    *[]gitLabDiffFile `json:"diffs"`
}

type gitLabDiffFile struct {
	OldPath   string  `json:"old_path"`
	NewPath   string  `json:"new_path"`
	OldMode   string  `json:"a_mode"`
	NewMode   string  `json:"b_mode"`
	Diff      *string `json:"diff"`
	New       bool    `json:"new_file"`
	Deleted   bool    `json:"deleted_file"`
	Renamed   bool    `json:"renamed_file"`
	Binary    bool    `json:"binary"`
	Collapsed bool    `json:"collapsed"`
	TooLarge  bool    `json:"too_large"`
}

func (mr *mergeRequest) isCurrentDiff(v diffVersion) bool {
	if v.HeadSHA != cmp.Or(mr.SHA, mr.DiffRefs.HeadSHA) || !validSHA(v.BaseSHA) {
		return false
	}
	if mr.DiffRefs.HeadSHA != v.HeadSHA {
		return true
	}
	return (mr.DiffRefs.BaseSHA == "" || mr.DiffRefs.BaseSHA == v.BaseSHA) &&
		(mr.DiffRefs.StartSHA == "" || mr.DiffRefs.StartSHA == v.StartSHA)
}

func (h *historyReader) diff(ctx context.Context, root string, version diffVersion) error {
	payload := envelope.MrDiffV1{
		MrNativeId: h.mr, BaseSha: version.BaseSHA, HeadSha: version.HeadSHA,
		Completeness: envelope.MrDiffV1CompletenessPartial, Files: []envelope.MrDiffFile{},
	}
	var response diffDetails
	err := h.get(ctx, "merge_request_diff", root+"/versions/"+id(version.ID), &response)
	switch {
	case err == nil:
		if response.ID == version.ID && id(response.MRID) == h.mr && response.HeadSHA == version.HeadSHA &&
			response.BaseSHA == version.BaseSHA && response.StartSHA == version.StartSHA {
			payload.Files, payload.Completeness = response.summary()
		}
	case hasCode(err, adaptersdk.AdapterErrorCodeNotFound), hasCode(err, adaptersdk.AdapterErrorCodeAuth),
		hasCode(err, adaptersdk.AdapterErrorCodeInvalidResponse):
		// The MR and its revision are still useful if the patch cannot be read.
	default:
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if len(raw) > _maxDiffPayloadBytes {
		payload.Files, payload.Completeness = payload.Files[:0], envelope.MrDiffV1CompletenessPartial
	}
	return addRecord(h.page, "mr_diff", "mr_diff/1", "object", h.mr, id(version.ID), nil, nil, h.observed, payload)
}

func (d *diffDetails) summary() ([]envelope.MrDiffFile, envelope.MrDiffV1Completeness) {
	var files []gitLabDiffFile
	if d.Files != nil {
		files = *d.Files
	}
	expected, err := strconv.Atoi(d.RealSize)
	complete := d.Files != nil && d.State == "collected" && err == nil && expected == len(files) && len(files) <= _maxDiffFiles
	out := make([]envelope.MrDiffFile, 0, min(len(files), _maxDiffFiles))
	seen := make(map[[2]string]bool, cap(out))
	for i := range files[:min(len(files), _maxDiffFiles)] {
		f := &files[i]
		key := [2]string{f.OldPath, f.NewPath}
		if !diffPath(f.OldPath) || !diffPath(f.NewPath) || seen[key] || f.New && f.Deleted ||
			f.Renamed && (f.New || f.Deleted || f.OldPath == f.NewPath) {
			complete = false
			continue
		}
		seen[key] = true
		summary := f.summary()
		complete = complete && summary.AddedLines != nil && !f.Collapsed && !f.TooLarge
		out = append(out, summary)
	}
	slices.SortFunc(out, func(a, b envelope.MrDiffFile) int {
		return cmp.Or(cmp.Compare(a.NewPath, b.NewPath), cmp.Compare(a.OldPath, b.OldPath))
	})
	if complete {
		return out, envelope.MrDiffV1CompletenessComplete
	}
	return out, envelope.MrDiffV1CompletenessPartial
}

func diffPath(path string) bool {
	return len(path) <= 4096 && envelope.ValidMRDiffPath(path)
}

func (f *gitLabDiffFile) summary() envelope.MrDiffFile {
	out := envelope.MrDiffFile{
		OldPath: f.OldPath, NewPath: f.NewPath, NewFile: f.New, DeletedFile: f.Deleted, RenamedFile: f.Renamed,
		Binary: f.Binary, HeadRanges: []envelope.MrDiffRange{},
	}
	if f.Binary {
		out.AddedLines, out.DeletedLines = new(0), new(0)
		return out
	}
	if f.Collapsed || f.TooLarge || f.Diff == nil {
		return out
	}
	if *f.Diff == "" && (f.Renamed || f.New || f.Deleted || f.OldMode != f.NewMode) {
		out.AddedLines, out.DeletedLines = new(0), new(0)
		return out
	}
	patch, ok := summarizePatch(*f.Diff)
	if ok && (!f.Deleted || patch.added == 0) && (!f.New || patch.deleted == 0) {
		out.Binary = patch.binary
		out.AddedLines, out.DeletedLines = &patch.added, &patch.deleted
		out.HeadRanges = patch.ranges
	}
	return out
}

var _hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?:.*)$`)

type patchSummary struct {
	added, deleted int
	binary         bool
	ranges         []envelope.MrDiffRange
}

type hunk struct {
	oldLeft, newLeft int
	oldLine, newLine int
}

func (h *hunk) complete() bool { return h.oldLeft == 0 && h.newLeft == 0 }

func summarizePatch(patch string) (patchSummary, bool) {
	out := patchSummary{ranges: []envelope.MrDiffRange{}}
	var h hunk
	var started, metadata bool
	for line := range strings.SplitSeq(strings.TrimSuffix(patch, "\n"), "\n") {
		if strings.HasPrefix(line, "@@") {
			next, ok := parseHunk(line)
			if !ok || !h.complete() || started && (next.oldLine < h.oldLine || next.newLine < h.newLine) {
				return patchSummary{}, false
			}
			h, started = next, true
			continue
		}
		if !started {
			if binaryPatchLine(line) {
				out.binary = true
				return out, true
			}
			if patchMetadata(line) {
				metadata = metadata || metadataOnlyChange(line)
				continue
			}
			return patchSummary{}, false
		}
		if !out.line(&h, line) || len(out.ranges) > _maxHeadRanges {
			return patchSummary{}, false
		}
	}
	return out, h.complete() && (started || metadata)
}

func binaryPatchLine(line string) bool {
	return line == "GIT binary patch" || strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ")
}

func metadataOnlyChange(line string) bool {
	return strings.HasPrefix(line, "old mode ") || strings.HasPrefix(line, "new mode ") ||
		strings.HasPrefix(line, "new file mode ") || strings.HasPrefix(line, "deleted file mode ") ||
		strings.HasPrefix(line, "rename from ") || strings.HasPrefix(line, "rename to ")
}

func patchMetadata(line string) bool {
	return metadataOnlyChange(line) || strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "similarity index ") || strings.HasPrefix(line, "dissimilarity index ") ||
		strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")
}

func parseHunk(line string) (hunk, bool) {
	parts := _hunkHeader.FindStringSubmatch(line)
	if parts == nil {
		return hunk{}, false
	}
	oldStart, oldCount, oldOK := parseHunkRange(parts[1], parts[2])
	newStart, newCount, newOK := parseHunkRange(parts[3], parts[4])
	return hunk{oldLine: oldStart, oldLeft: oldCount, newLine: newStart, newLeft: newCount}, oldOK && newOK && (oldCount != 0 || newCount != 0)
}

func parseHunkRange(start, count string) (line int, n int, ok bool) {
	parsedStart, startErr := strconv.ParseInt(start, 10, 32)
	parsedCount, countErr := strconv.ParseInt(cmp.Or(count, "1"), 10, 32)
	if startErr != nil || countErr != nil || parsedCount > 0 && parsedStart == 0 || parsedStart+parsedCount > 1<<31 {
		return 0, 0, false
	}
	return int(parsedStart), int(parsedCount), true
}

func (p *patchSummary) line(h *hunk, line string) bool {
	if line == `\ No newline at end of file` {
		return true
	}
	if line == "" {
		return false
	}
	switch line[0] {
	case '+':
		p.add(h.newLine)
		h.newLine++
		h.newLeft--
	case '-':
		p.deleted++
		h.oldLine++
		h.oldLeft--
	case ' ':
		h.oldLine++
		h.newLine++
		h.oldLeft--
		h.newLeft--
	default:
		return false
	}
	return h.oldLeft >= 0 && h.newLeft >= 0
}

func (p *patchSummary) add(line int) {
	p.added++
	if n := len(p.ranges); n > 0 && p.ranges[n-1].To+1 == line {
		p.ranges[n-1].To = line
		return
	}
	p.ranges = append(p.ranges, envelope.MrDiffRange{From: line, To: line})
}
