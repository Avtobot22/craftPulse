package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"
)

type Window struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Model struct {
	SchemaVersion string    `json:"schema_version"`
	Seed          uint64    `json:"seed"`
	Period        Window    `json:"period"`
	AsOf          string    `json:"as_of"`
	PeriodReason  string    `json:"period_reason"`
	PageSize      int       `json:"page_size"`
	People        []Person  `json:"people"`
	Volumes       []Volume  `json:"volumes"`
	Variants      []Variant `json:"variants"`
}

type Volume struct {
	OrganizationID string `json:"organization_id"`
	InstanceID     string `json:"instance_id"`
	AreaID         string `json:"area_id"`
	Kind           string `json:"kind"`
	Count          int    `json:"count"`
	Series         string `json:"series"`
	Restricted     bool   `json:"restricted"`
}

func Generate(seed uint64, asOf string) (*Model, error) {
	end, err := parseSecond(asOf)
	if err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !end.After(start) {
		return nil, fmt.Errorf("as_of must be after period start")
	}
	rng := rand.New(rand.NewPCG(seed, 21))
	return &Model{
		SchemaVersion: "1", Seed: seed,
		Period:       Window{From: start.Format(time.RFC3339), To: end.UTC().Format(time.RFC3339)},
		AsOf:         end.UTC().Format(time.RFC3339),
		PeriodReason: fmt.Sprintf("Конец периода выбран %s вместо предложенного 2026-11-01: граница должна покрывать время загрузки источников. По умолчанию 2026-12-01 покрывает ноябрьскую загрузку. observed_time должен быть раньше выбранной границы.", end.UTC().Format(time.RFC3339)),
		PageSize:     10, People: generatePeople(rng), Volumes: volumes(), Variants: Catalog(),
	}, nil
}

func Hash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode hash input: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func volumes() []Volume {
	var result []Volume
	add := func(org, instance, area, kind string, count int, restricted bool, series string) {
		result = append(result, Volume{OrganizationID: org, InstanceID: instance, AreaID: area, Kind: kind, Count: count, Restricted: restricted, Series: series})
	}
	for _, area := range []string{"jira-project-a", "jira-project-b"} {
		add("org-1", "jira-dc-1", area, "task", 40, false, "response")
		add("org-1", "jira-dc-1", area, "task_event", 400, false, "response")
	}
	add("org-1", "universal-1", "import-project", "task", 20, false, "response")
	add("org-1", "universal-1", "import-project", "task_event", 200, false, "response")
	for i, instance := range []string{"gitlab-1", "github-1"} {
		count := 6 - i*2
		area := fmt.Sprintf("repo-%d", i+1)
		add("org-1", instance, area, "merge_request", count, i == 1, "response")
		add("org-1", instance, area, "ci_run", count*2, i == 1, "response")
		add("org-1", instance, area, "git_document", 25, i == 1, "response")
	}
	add("org-1", "confluence-dc-1", "wiki-space", "wiki_document", 10, false, "response")
	add("org-1", "jenkins-1", "job-series", "ci_run", 6, false, "adapter")
	add("org-2", "jira-dc-1", "private-project", "task", 1, true, "isolation")
	add("org-2", "jira-dc-1", "private-project", "task_event", 10, true, "isolation")
	for _, kind := range []string{"merge_request", "ci_run", "git_document"} {
		add("org-2", "gitlab-1", "private-repo", kind, 1, true, "isolation")
	}
	add("org-2", "confluence-dc-1", "private-space", "wiki_document", 1, true, "isolation")
	return result
}
