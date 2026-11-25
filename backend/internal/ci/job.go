package ci

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

// TokenEnv names the variable with the upload token of the project.
const TokenEnv = "CRAFTPULSE_UPLOAD_TOKEN" //nolint:gosec // the name of the variable, not its value

var (
	_sha          = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	_nativeID     = regexp.MustCompile(`^\d{1,20}$`)
	_defaultPorts = map[string]string{"https": "443", "http": "80"}
)

// Job holds the step settings and the coordinates of the current CI job.
type Job struct {
	Site           *url.URL
	Provider       envelope.SourceProvider
	AttemptNo      int
	BuildURL       string
	Token          string
	InstanceKey    string
	ProjectID      string
	ProjectPath    string
	PipelineID     string
	JobID          string
	SHA            string
	RefName        string
	MergeRequestID string
	WorkDir        string
}

// JobFromEnv reads CRAFTPULSE_URL, CRAFTPULSE_UPLOAD_TOKEN and the predefined CI variables. Errors
// name a variable, never its value.
func JobFromEnv(getenv func(string) string) (Job, error) {
	j := Job{
		Provider: envelope.SourceProviderGitlab, Token: getenv(TokenEnv), ProjectID: getenv("CI_PROJECT_ID"), ProjectPath: getenv("CI_PROJECT_PATH"),
		PipelineID: getenv("CI_PIPELINE_ID"), JobID: getenv("CI_JOB_ID"), SHA: getenv("CI_COMMIT_SHA"),
		RefName: getenv("CI_COMMIT_REF_NAME"), MergeRequestID: getenv("CI_MERGE_REQUEST_ID"),
		InstanceKey: getenv("CRAFTPULSE_INSTANCE_KEY"), WorkDir: getenv("CRAFTPULSE_WORK_DIR"),
	}
	site, err := siteURL(getenv("CRAFTPULSE_URL"))
	if err != nil {
		return Job{}, err
	}
	j.Site = site
	if getenv("GITHUB_ACTIONS") == "true" || getenv("JENKINS_URL") != "" || (getenv("JOB_NAME") != "" && getenv("BUILD_URL") != "") {
		if j.Token == "" {
			return Job{}, errors.New(TokenEnv + " is not set")
		}
		return j.otherCI(getenv)
	}
	var problems []error
	if j.Token == "" {
		problems = append(problems, errors.New(TokenEnv+" is not set"))
	}
	for name, value := range map[string]string{"CI_PROJECT_ID": j.ProjectID, "CI_PIPELINE_ID": j.PipelineID, "CI_JOB_ID": j.JobID} {
		if !_nativeID.MatchString(value) {
			problems = append(problems, fmt.Errorf("%s must be a GitLab ID", name))
		}
	}
	if j.MergeRequestID != "" && !_nativeID.MatchString(j.MergeRequestID) {
		problems = append(problems, errors.New("CI_MERGE_REQUEST_ID must be a GitLab ID"))
	}
	if !_sha.MatchString(j.SHA) {
		problems = append(problems, errors.New("CI_COMMIT_SHA must be a full lowercase commit SHA"))
	}
	if j.InstanceKey == "" {
		j.InstanceKey = gitlabInstanceKey(getenv("CI_SERVER_HOST"), getenv("CI_SERVER_PORT"), getenv("CI_SERVER_PROTOCOL"))
	}
	if j.InstanceKey == "" {
		problems = append(problems, errors.New("CI_SERVER_HOST is not set"))
	}
	if j.WorkDir == "" {
		j.WorkDir = filepath.Join(os.TempDir(), "craftpulse-ci-"+j.JobID)
	}
	return j, errors.Join(problems...)
}

func (j Job) otherCI(getenv func(string) string) (Job, error) {
	var address string
	if getenv("GITHUB_ACTIONS") == "true" {
		j.Provider = envelope.SourceProviderGithub
		j.ProjectID, j.ProjectPath = getenv("GITHUB_REPOSITORY_ID"), getenv("GITHUB_REPOSITORY")
		j.PipelineID, j.JobID = getenv("GITHUB_RUN_ID"), getenv("GITHUB_JOB")
		j.SHA, j.RefName = getenv("GITHUB_SHA"), getenv("GITHUB_REF")
		j.AttemptNo, _ = strconv.Atoi(getenv("GITHUB_RUN_ATTEMPT"))
		address = getenv("GITHUB_SERVER_URL")
		if address == "" {
			address = "https://github.com"
		}
		if !_nativeID.MatchString(j.ProjectID) || !_nativeID.MatchString(j.PipelineID) || j.AttemptNo < 1 || j.JobID == "" {
			return Job{}, errors.New("GITHUB_REPOSITORY_ID, GITHUB_RUN_ID, GITHUB_RUN_ATTEMPT and GITHUB_JOB must identify the Actions job")
		}
		event, err := readActionsEvent(getenv("GITHUB_EVENT_PATH"))
		if err != nil {
			return Job{}, err
		}
		if n := event.PullRequest.Number; n > 0 {
			j.MergeRequestID = strconv.Itoa(n)
		}
	} else {
		j.Provider = envelope.SourceProviderJenkins
		j.JobID, j.PipelineID, j.BuildURL = getenv("JOB_NAME"), getenv("BUILD_NUMBER"), getenv("BUILD_URL")
		j.ProjectID, j.ProjectPath = j.JobID, j.JobID
		j.SHA, j.RefName, j.MergeRequestID = getenv("GIT_COMMIT"), getenv("BRANCH_NAME"), getenv("CHANGE_ID")
		if j.RefName == "" {
			j.RefName = getenv("GIT_BRANCH")
		}
		if j.MergeRequestID != "" && getenv("CHANGE_BRANCH") != "" {
			j.RefName = getenv("CHANGE_BRANCH")
		}
		number, err := strconv.Atoi(j.PipelineID)
		if err != nil || number < 1 || strconv.Itoa(number) != j.PipelineID || j.JobID == "" || j.BuildURL == "" {
			return Job{}, errors.New("JOB_NAME, BUILD_NUMBER and BUILD_URL must identify the Jenkins build")
		}
		address = getenv("JENKINS_URL")
		if address == "" {
			address = j.BuildURL
		}
	}
	if !_sha.MatchString(j.SHA) {
		return Job{}, errors.New("GITHUB_SHA or GIT_COMMIT must be a full lowercase commit SHA")
	}
	if j.InstanceKey == "" {
		u, err := siteURL(address)
		if err != nil {
			return Job{}, errors.New("GITHUB_SERVER_URL or BUILD_URL must be an absolute CI URL")
		}
		j.InstanceKey = gitlabInstanceKey(u.Hostname(), u.Port(), u.Scheme)
	}
	if j.WorkDir == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s/%d/%s", j.Provider, j.ProjectID, j.PipelineID, j.AttemptNo, j.JobID)))
		j.WorkDir = filepath.Join(os.TempDir(), fmt.Sprintf("craftpulse-ci-%x", sum))
	}
	return j, nil
}

// CheckoutRoot is the repository workspace supplied by the CI runner.
func CheckoutRoot(getenv func(string) string) string {
	switch {
	case getenv("GITHUB_ACTIONS") == "true":
		return getenv("GITHUB_WORKSPACE")
	case getenv("JOB_NAME") != "":
		return getenv("WORKSPACE")
	default:
		return getenv("CI_PROJECT_DIR")
	}
}

// HeadSHA is the SHA of the checkout being scanned.
func HeadSHA(getenv func(string) string) string {
	switch {
	case getenv("GITHUB_ACTIONS") == "true":
		return getenv("GITHUB_SHA")
	case getenv("JOB_NAME") != "":
		return getenv("GIT_COMMIT")
	default:
		return getenv("CI_COMMIT_SHA")
	}
}

// BaseSHA is the comparison SHA supplied by the runner, when available.
func BaseSHA(getenv func(string) string) string {
	if getenv("GITHUB_ACTIONS") == "true" {
		event, _ := readActionsEvent(getenv("GITHUB_EVENT_PATH"))
		if event.PullRequest.Base.SHA != "" {
			return event.PullRequest.Base.SHA
		}
		return event.Before
	}
	if getenv("JOB_NAME") != "" {
		return getenv("GIT_PREVIOUS_COMMIT")
	}
	if base := getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA"); base != "" {
		return base
	}
	return getenv("CI_COMMIT_BEFORE_SHA")
}

// siteURL accepts the site address over HTTPS; plain HTTP only reaches a loopback test server.
func siteURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("CRAFTPULSE_URL is not set")
	}
	u, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("CRAFTPULSE_URL must be an absolute URL without credentials, query or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && (u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()))) {
		return nil, errors.New("CRAFTPULSE_URL must use https")
	}
	return u, nil
}

// gitlabInstanceKey is the host, with the port when it is not the default one: the key the GitLab
// connection gets from the normalized base address (TZ 12.4).
func gitlabInstanceKey(host, port, scheme string) string {
	host = strings.ToLower(host)
	if host == "" || port == "" || port == _defaultPorts[scheme] {
		return host
	}
	return net.JoinHostPort(host, port)
}

// ci gives the coordinates of a tool run in the current pipeline. GitLab gives a new job ID at every retry,
// so the job ID is also the attempt; an empty job leaves the producer job unknown.
func (j *Job) ci(job string) *envelope.ScanCiCoordinates {
	c := &envelope.ScanCiCoordinates{
		Provider: envelope.ScanCiCoordinatesProviderGitlab, ProjectNativeId: j.ProjectID, RunNativeId: j.PipelineID,
		RefName: j.RefName,
	}
	switch j.Provider {
	case envelope.SourceProviderGithub:
		c.Provider = envelope.ScanCiCoordinatesProviderGithub
		c.JobNativeId, c.AttemptNo = optional(job), &j.AttemptNo
	case envelope.SourceProviderJenkins:
		c.Provider = envelope.ScanCiCoordinatesProviderJenkins
		c.JobNativeId = optional(job)
	default:
		if job != "" {
			c.JobNativeId, c.AttemptNativeId = &job, &job
		}
	}
	if j.MergeRequestID != "" {
		mr := j.MergeRequestID
		c.MrNativeId = &mr
	}
	return c
}
