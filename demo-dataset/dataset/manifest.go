package dataset

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"sort"
)

type SourceTime struct {
	UTC            string `json:"utc"`
	OriginalOffset string `json:"original_offset"`
}

type Identity struct {
	OrganizationID string `json:"organization_id"`
	InstanceID     string `json:"instance_id"`
	ObjectKind     string `json:"object_kind"`
	NativeNS       string `json:"native_ns"`
	NativeID       string `json:"native_id"`
}

type Binding struct {
	DatasetID    string     `json:"dataset_id"`
	Identity     Identity   `json:"identity"`
	SHA          string     `json:"sha"`
	EventTime    SourceTime `json:"event_time"`
	ObservedTime SourceTime `json:"observed_time"`
}

type SourceManifest struct {
	SchemaVersion  string            `json:"schema_version"`
	OrganizationID string            `json:"organization_id"`
	InstanceID     string            `json:"instance_id"`
	SourceURL      string            `json:"source_url"`
	Edition        string            `json:"edition"`
	Version        string            `json:"version"`
	Queries        []string          `json:"queries"`
	Scopes         []string          `json:"scopes"`
	Backfill       Window            `json:"backfill"`
	AsOf           string            `json:"as_of"`
	Completeness   string            `json:"completeness"`
	Pages          []Page            `json:"pages"`
	Objects        []Binding         `json:"objects"`
	Versions       map[string]string `json:"versions"`
	Hashes         map[string]string `json:"hashes"`
	CleanedFiles   map[string]string `json:"cleaned_files"`
	ExpectedFiles  []string          `json:"expected_files"`
}

type Page struct {
	Query      string `json:"query"`
	Cursor     string `json:"cursor"`
	NextCursor string `json:"next_cursor"`
	Count      int    `json:"count"`
	Complete   bool   `json:"complete"`
}

type VariantObjects struct {
	VariantID string   `json:"variant_id"`
	ObjectIDs []string `json:"object_ids"`
}

type ObjectFile struct {
	SchemaVersion string           `json:"schema_version"`
	Objects       []ModelObject    `json:"objects"`
	Variants      []VariantObjects `json:"variants"`
}

type ModelObject struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Kind           string `json:"kind"`
}

type Manifest struct {
	SchemaVersion string           `json:"schema_version"`
	ModelHash     string           `json:"model_hash"`
	Period        Window           `json:"period"`
	AsOf          string           `json:"as_of"`
	Sources       []SourceManifest `json:"sources"`
	Variants      []VariantObjects `json:"variants"`
}

func (s *SourceManifest) Validate() error {
	if s.SchemaVersion != "1" || s.OrganizationID == "" || s.InstanceID == "" || s.Edition == "" || s.Version == "" || len(s.Queries) == 0 || len(s.Scopes) == 0 || s.Pages == nil || s.Objects == nil || s.CleanedFiles == nil || s.ExpectedFiles == nil {
		return fmt.Errorf("source manifest is missing required metadata")
	}
	parsed, err := url.Parse(s.SourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("source_url must be HTTPS without credentials, query or fragment")
	}
	if err := validateWindow(s.Backfill); err != nil {
		return err
	}
	asOf, err := parseSecond(s.AsOf)
	if err != nil {
		return err
	}
	to, err := parseSecond(s.Backfill.To)
	if err != nil {
		return err
	}
	if to.After(asOf) {
		return fmt.Errorf("backfill exceeds as_of")
	}
	if !slices.Contains([]string{"complete", "partial", "unknown"}, s.Completeness) {
		return fmt.Errorf("invalid completeness")
	}
	for _, key := range []string{"mapping", "calculation", "parser", "tool"} {
		if s.Versions[key] == "" {
			return fmt.Errorf("versions requires %s", key)
		}
	}
	for _, key := range []string{"rules", "config", "db"} {
		if !validHash(s.Hashes[key]) {
			return fmt.Errorf("hashes requires SHA-256 for %s", key)
		}
	}
	for _, hash := range s.CleanedFiles {
		if !validHash(hash) {
			return fmt.Errorf("cleaned file hash must be SHA-256")
		}
	}
	for i, page := range s.Pages {
		if !slices.Contains(s.Queries, page.Query) || page.Count < 0 || page.Count > 10 {
			return fmt.Errorf("page %d: invalid query or count", i)
		}
		if s.Completeness == "complete" && !page.Complete {
			return fmt.Errorf("complete source contains incomplete page")
		}
	}
	seen := make(map[Identity]bool)
	for i, object := range s.Objects {
		identity := object.Identity
		if object.DatasetID == "" || identity.OrganizationID != s.OrganizationID || identity.InstanceID != s.InstanceID || identity.ObjectKind == "" || identity.NativeID == "" || seen[identity] {
			return fmt.Errorf("object %d: invalid or duplicate identity", i)
		}
		seen[identity] = true
		for _, value := range []SourceTime{object.EventTime, object.ObservedTime} {
			timestamp, err := parseSecond(value.UTC)
			if err != nil || !validOffset(value.OriginalOffset) || timestamp.Format("Z07:00") != "Z" {
				return fmt.Errorf("object %d: invalid source time", i)
			}
			if !timestamp.Before(asOf) {
				return fmt.Errorf("object %d: time is outside as_of", i)
			}
		}
	}
	return nil
}

func Assemble(model *Model, sources []SourceManifest, files []ObjectFile) (*Manifest, error) {
	hash, err := Hash(model)
	if err != nil {
		return nil, err
	}
	if err := validateWindow(model.Period); err != nil {
		return nil, err
	}
	if model.SchemaVersion != "1" || model.Period.To != model.AsOf {
		return nil, fmt.Errorf("invalid model version or as_of")
	}
	objects := make(map[string]ModelObject)
	variants := make(map[string]map[string]bool)
	for _, variant := range model.Variants {
		variants[variant.ID] = make(map[string]bool)
	}
	for _, file := range files {
		if file.SchemaVersion != "1" || file.Objects == nil || file.Variants == nil {
			return nil, fmt.Errorf("invalid object file")
		}
		for _, object := range file.Objects {
			if _, exists := objects[object.ID]; exists {
				return nil, fmt.Errorf("duplicate model object id")
			}
			if object.ID == "" || object.OrganizationID == "" || object.Kind == "" {
				return nil, fmt.Errorf("invalid model object")
			}
			objects[object.ID] = object
		}
	}
	for _, file := range files {
		for _, binding := range file.Variants {
			members, exists := variants[binding.VariantID]
			if !exists {
				return nil, fmt.Errorf("unknown variant id")
			}
			for _, id := range binding.ObjectIDs {
				if _, exists := objects[id]; !exists {
					return nil, fmt.Errorf("variant refers to unknown model object")
				}
				members[id] = true
			}
		}
	}
	seenSources := make(map[[2]string]bool)
	seenBindings := make(map[[3]string]bool)
	sources = slices.Clone(sources)
	for i := range sources {
		source := &sources[i]
		if err := source.Validate(); err != nil {
			return nil, fmt.Errorf("source %d: %w", i, err)
		}
		if source.AsOf != model.AsOf {
			return nil, fmt.Errorf("source %d: as_of differs from model", i)
		}
		if source.Backfill != model.Period {
			return nil, fmt.Errorf("source %d: backfill differs from model", i)
		}
		key := [2]string{source.OrganizationID, source.InstanceID}
		if seenSources[key] {
			return nil, fmt.Errorf("duplicate source manifest")
		}
		seenSources[key] = true
		for _, binding := range source.Objects {
			object, exists := objects[binding.DatasetID]
			if !exists || object.OrganizationID != binding.Identity.OrganizationID || object.Kind != binding.Identity.ObjectKind {
				return nil, fmt.Errorf("source %d: binding disagrees with model object", i)
			}
			key := [3]string{source.OrganizationID, source.InstanceID, binding.DatasetID}
			if seenBindings[key] {
				return nil, fmt.Errorf("source %d: repeated dataset binding", i)
			}
			seenBindings[key] = true
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].OrganizationID != sources[j].OrganizationID {
			return sources[i].OrganizationID < sources[j].OrganizationID
		}
		return sources[i].InstanceID < sources[j].InstanceID
	})
	summary := make([]VariantObjects, 0, len(variants))
	for id, members := range variants {
		ids := make([]string, 0, len(members))
		for member := range members {
			ids = append(ids, member)
		}
		sort.Strings(ids)
		summary = append(summary, VariantObjects{VariantID: id, ObjectIDs: ids})
	}
	sort.Slice(summary, func(i, j int) bool { return summary[i].VariantID < summary[j].VariantID })
	if sources == nil {
		sources = []SourceManifest{}
	}
	return &Manifest{SchemaVersion: "1", ModelHash: hash, Period: model.Period, AsOf: model.AsOf, Sources: sources, Variants: summary}, nil
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
