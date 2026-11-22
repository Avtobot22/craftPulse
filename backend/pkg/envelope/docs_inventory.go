package envelope

import "github.com/google/uuid"

// DocsInventory is the docs_json/1 report of a documentation check at a Git commit.
type DocsInventory struct {
	FormatVersion string        `json:"format_version"`
	RootID        *uuid.UUID    `json:"root_id"`
	PathPrefix    string        `json:"path_prefix"`
	AnchorRule    string        `json:"anchor_rule"`
	OwnerRule     string        `json:"owner_front_matter_key,omitempty"`
	Checks        []string      `json:"checks,omitempty"`
	Documents     []DocsEntry   `json:"documents"`
	Links         *DocsLinkStat `json:"links"`
}

type DocsEntry struct {
	Document  DocumentV1 `json:"document"`
	Owner     *string    `json:"owner"`
	SpecValid *bool      `json:"spec_valid"`
}

type DocsLinkStat struct {
	Checked  int `json:"checked"`
	Broken   int `json:"broken"`
	External int `json:"external"`
}

// DocsLinkTarget is safe SARIF evidence for a local link. It omits query parameters.
type DocsLinkTarget struct {
	TargetPath string `json:"target_path"`
	Anchor     string `json:"anchor,omitempty"`
}
