package ci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

// ActionsTokenEnv names the read-only Actions API token used to resolve the numeric job ID.
const ActionsTokenEnv = "CRAFTPULSE_ACTIONS_TOKEN" //nolint:gosec // environment variable name

var _repositorySlug = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type actionsEvent struct {
	Before      string `json:"before"`
	PullRequest struct {
		Number int `json:"number"`
		Base   struct {
			SHA string `json:"sha"`
		} `json:"base"`
	} `json:"pull_request"`
}

func readActionsEvent(path string) (actionsEvent, error) {
	var event actionsEvent
	if path == "" {
		return event, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return event, errors.New("cannot open GITHUB_EVENT_PATH")
	}
	defer f.Close()
	if err := json.NewDecoder(io.LimitReader(f, 25<<20)).Decode(&event); err != nil {
		return event, errors.New("cannot decode GITHUB_EVENT_PATH")
	}
	return event, nil
}

// ActionsAttemptID returns the source key <run attempt>/<numeric job ID>.
// GITHUB_JOB is the workflow job key. Generated snippets use it as the job display name.
func (j *Job) ActionsAttemptID(ctx context.Context, getenv func(string) string) (string, error) {
	return j.actionsAttemptID(ctx, getenv, nil)
}

func (j *Job) actionsAttemptID(ctx context.Context, getenv func(string) string, transport http.RoundTripper) (string, error) {
	token := getenv(ActionsTokenEnv)
	if token == "" {
		return "", nil
	}
	api, err := siteURL(getenv("GITHUB_API_URL"))
	if err != nil || api.Scheme != "https" {
		return "", errors.New("GITHUB_API_URL must identify the HTTPS Actions API")
	}
	if !_repositorySlug.MatchString(j.ProjectPath) {
		return "", errors.New("GITHUB_REPOSITORY must be an owner/repository pair")
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	name := getenv("CRAFTPULSE_ACTIONS_JOB_NAME")
	if name == "" {
		name = j.JobID
	}
	var found int64
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("%s/repos/%s/actions/runs/%s/attempts/%d/jobs?per_page=100&page=%d", api, j.ProjectPath, j.PipelineID, j.AttemptNo, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
		if err != nil {
			return "", errors.New("cannot construct Actions jobs request")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		jobs, err := actionsJobs(client, req)
		if err != nil {
			return "", err
		}
		for _, job := range jobs {
			if job.Name == name {
				if found != 0 {
					return "", errors.New("Actions job display name is ambiguous")
				}
				found = job.ID
			}
		}
		if len(jobs) < 100 {
			break
		}
	}
	if found <= 0 {
		return "", errors.New("current job not found in Actions run attempt")
	}
	return fmt.Sprintf("%d/%d", j.AttemptNo, found), nil
}

type actionsJob struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func actionsJobs(client *http.Client, req *http.Request) ([]actionsJob, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("cannot read Actions jobs")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Actions jobs request returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Jobs []actionsJob `json:"jobs"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, errors.New("cannot decode Actions jobs")
	}
	return result.Jobs, nil
}
