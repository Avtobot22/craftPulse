package dataset

// ChangeFile describes actions, not fabricated source timestamps or native IDs.
type ChangeFile struct {
	SchemaVersion string           `json:"schema_version"`
	Seed          uint64           `json:"seed"`
	Period        Window           `json:"period"`
	AsOf          string           `json:"as_of"`
	MergeRequests []MergeRequest   `json:"merge_requests"`
	Runs          []CIRun          `json:"runs"`
	Branches      []CIBranch       `json:"branches"`
	Variants      []VariantObjects `json:"variants"`
}

type MergeRequest struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	InstanceID     string       `json:"instance_id"`
	RepositoryID   string       `json:"repository_id"`
	Branch         string       `json:"branch"`
	Head           string       `json:"head"`
	Title          string       `json:"title"`
	AuthorID       string       `json:"author_id"`
	InitialState   string       `json:"initial_state"`
	FinalState     string       `json:"final_state"`
	Reviewers      []string     `json:"reviewers"`
	Events         []MREvent    `json:"events"`
	TaskLinks      []MRTaskLink `json:"task_links"`
}

type MREvent struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	ActorID string `json:"actor_id"`
	Body    string `json:"body"`
}

type MRTaskLink struct {
	Kind    string   `json:"kind"`
	Key     string   `json:"key"`
	TaskIDs []string `json:"task_ids"`
}

type CIBranch struct {
	RepositoryID     string `json:"repository_id"`
	Name             string `json:"name"`
	Commit           Commit `json:"commit"`
	DefinitionPath   string `json:"definition_path"`
	DefinitionSHA256 string `json:"definition_sha256"`
}

type CIRun struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	InstanceID        string  `json:"instance_id"`
	Provider          string  `json:"provider"`
	RepositoryID      string  `json:"repository_id"`
	MergeRequestID    string  `json:"merge_request_id"`
	Branch            string  `json:"branch"`
	SHA               string  `json:"sha"`
	InitialStatus     string  `json:"initial_status"`
	FinalStatus       string  `json:"final_status"`
	ExecutionManifest bool    `json:"execution_manifest"`
	Jobs              []CIJob `json:"jobs"`
}

type CIJob struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Stage             string            `json:"stage"`
	Axes              map[string]string `json:"axes"`
	ConfigurationHash string            `json:"configuration_hash"`
	AllowFailure      bool              `json:"allow_failure"`
	InitialStatus     string            `json:"initial_status"`
	Attempts          []CIAttempt       `json:"attempts"`
}

type CIAttempt struct {
	ID      string     `json:"id"`
	Number  int        `json:"number"`
	Outcome string     `json:"outcome"`
	Reports []CIReport `json:"reports"`
}

type CIReport struct {
	ID              string `json:"id"`
	Path            string `json:"path"`
	Format          string `json:"format"`
	SHA             string `json:"sha"`
	TestCaseID      string `json:"test_case_id"`
	CoveredLines    int    `json:"covered_lines"`
	TotalLines      int    `json:"total_lines"`
	CoveredBranches int    `json:"covered_branches"`
	TotalBranches   int    `json:"total_branches"`
}

type LoadPlan struct {
	SchemaVersion string       `json:"schema_version"`
	ModelHash     string       `json:"model_hash"`
	Actions       []LoadAction `json:"actions"`
}

type LoadAction struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organization_id"`
	InstanceID     string            `json:"instance_id"`
	Kind           string            `json:"kind"`
	ObjectID       string            `json:"object_id"`
	ActorID        string            `json:"actor_id"`
	DependsOn      []string          `json:"depends_on"`
	Parameters     map[string]string `json:"parameters"`
	Expected       string            `json:"expected"`
	Bindings       []string          `json:"bindings"`
}

type ChangeSet struct {
	File *ChangeFile `json:"file"`
	Plan *LoadPlan   `json:"plan"`
}

func (s *ChangeSet) Objects() ObjectFile {
	out := ObjectFile{SchemaVersion: "1", Objects: []ModelObject{}, Variants: s.File.Variants}
	add := func(id, org, kind string) {
		out.Objects = append(out.Objects, ModelObject{ID: id, OrganizationID: org, Kind: kind})
	}
	for _, mr := range s.File.MergeRequests {
		add(mr.ID, mr.OrganizationID, "merge_request")
		for _, event := range mr.Events {
			add(event.ID, mr.OrganizationID, "mr_event")
		}
	}
	for _, run := range s.File.Runs {
		add(run.ID, run.OrganizationID, "ci_run")
		for _, job := range run.Jobs {
			add(job.ID, run.OrganizationID, "ci_job")
			for _, a := range job.Attempts {
				add(a.ID, run.OrganizationID, "ci_attempt")
				for _, r := range a.Reports {
					add(r.ID, run.OrganizationID, "report")
				}
			}
		}
	}
	return out
}
