package gitlab

import (
	"slices"
	"testing"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func TestSummarizePatchCountsAndHeadRanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		patch          string
		added, deleted int
		ranges         []envelope.MrDiffRange
		binary         bool
	}{
		{
			"replacement and insertion", "@@ -1,4 +1,6 @@\n first\n-old\n+one\n+two\n middle\n+extra\n last\n", 3, 1,
			[]envelope.MrDiffRange{{From: 2, To: 3}, {From: 5, To: 5}},
			false,
		},
		{
			"new empty context", "@@ -0,0 +1 @@\n+only\n\\ No newline at end of file\n", 1, 0,
			[]envelope.MrDiffRange{{From: 1, To: 1}},
			false,
		},
		{"deletion", "@@ -1,2 +0,0 @@\n-first\n-last\n", 0, 2, nil, false},
		{
			"separate hunks", "@@ -5 +5 @@ title\n-old\n+new\n@@ -20,0 +21 @@\n+extra\n", 2, 1,
			[]envelope.MrDiffRange{{From: 5, To: 5}, {From: 21, To: 21}},
			false,
		},
		{"binary notice", "Binary files a/picture and b/picture differ\n", 0, 0, nil, true},
		{"mode only", "old mode 100644\nnew mode 100755\n", 0, 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, complete := summarizePatch(tt.patch)
			if !complete || got.added != tt.added || got.deleted != tt.deleted || got.binary != tt.binary || !slices.Equal(got.ranges, tt.ranges) {
				t.Fatalf("summary = %+v, complete = %t, want added=%d deleted=%d binary=%t ranges=%v",
					got, complete, tt.added, tt.deleted, tt.binary, tt.ranges)
			}
		})
	}
}

func TestSummarizePatchRejectsIncompleteEvidence(t *testing.T) {
	t.Parallel()
	for name, patch := range map[string]string{
		"hunk cut":          "@@ -1 +1,2 @@\n-old\n+first\n",
		"no changes":        "@@ -1 +1 @@\n context only\n",
		"overlapping hunks": "@@ -1 +1 @@\n-old\n+new\n@@ -1 +1 @@\n-old\n+again\n",
		"wrong line range":  "@@ -0,0 +2147483647,2 @@\n+x\n+y\n",
		"missing patch":     "",
		"extra data":        "@@ -1 +1 @@\n-old\n+new\n+unaccounted\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, complete := summarizePatch(patch)
			if complete {
				t.Fatal("incomplete patch was accepted")
			}
		})
	}
}
