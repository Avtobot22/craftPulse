package ci

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

// Job holds the step settings and the coordinates of the current GitLab CI job.
type Job struct {
	Site           *url.URL
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

// JobFromEnv reads CRAFTPULSE_URL, CRAFTPULSE_UPLOAD_TOKEN and the predefined GitLab CI variables. Errors
// name a variable, never its value.
func JobFromEnv(getenv func(string) string) (Job, error) {
	j := Job{
		Token: getenv(TokenEnv), ProjectID: getenv("CI_PROJECT_ID"), ProjectPath: getenv("CI_PROJECT_PATH"),
		PipelineID: getenv("CI_PIPELINE_ID"), JobID: getenv("CI_JOB_ID"), SHA: getenv("CI_COMMIT_SHA"),
		RefName: getenv("CI_COMMIT_REF_NAME"), MergeRequestID: getenv("CI_MERGE_REQUEST_ID"),
		InstanceKey: getenv("CRAFTPULSE_INSTANCE_KEY"), WorkDir: getenv("CRAFTPULSE_WORK_DIR"),
	}
	site, err := siteURL(getenv("CRAFTPULSE_URL"))
	if err != nil {
		return Job{}, err
	}
	j.Site = site
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
	if job != "" {
		c.JobNativeId, c.AttemptNativeId = &job, &job
	}
	if j.MergeRequestID != "" {
		mr := j.MergeRequestID
		c.MrNativeId = &mr
	}
	return c
}
