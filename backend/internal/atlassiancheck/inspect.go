package atlassiancheck

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"
)

const _browseProjects = "BROWSE_PROJECTS"

type requestResult struct {
	Status int   `json:"status"`
	MS     int64 `json:"ms"`
}

type resourceResult struct {
	requestResult
	SiteURL          string `json:"site_url,omitempty"`
	JiraScopes       bool   `json:"jira_scopes"`
	ConfluenceScopes bool   `json:"confluence_scopes"`
}

type permissionResult struct {
	requestResult
	Target string `json:"target"`
	Browse *bool  `json:"browse,omitempty"`
}

type inspection struct {
	Objects     []accessResult     `json:"objects"`
	Permissions []permissionResult `json:"permissions"`
	Resource    resourceResult     `json:"resource"`
}

func (s *Server) inspect(ctx context.Context, role string) (inspection, error) {
	objects, err := s.checkObjects(ctx, role, true)
	if err != nil {
		return inspection{}, err
	}
	access := s.state.Tokens[role].Access
	resource, err := s.provider.inspectResource(ctx, access)
	if err != nil {
		return inspection{}, err
	}
	permissions := make([]permissionResult, 0, 3)
	for _, target := range []string{"projectKey=CPA", "projectKey=CPB", "issueKey=CPA-1"} {
		check, err := s.provider.inspectPermission(ctx, access, target)
		if err != nil {
			return inspection{}, err
		}
		permissions = append(permissions, check)
	}
	return inspection{Objects: objects, Resource: resource, Permissions: permissions}, nil
}

func (p provider) inspectResource(ctx context.Context, access string) (resourceResult, error) {
	start := time.Now()
	data, _, status, err := p.request(ctx, http.MethodGet,
		"https://api.atlassian.com/oauth/token/accessible-resources", access, nil)
	result := resourceResult{requestResult: requestResult{Status: status, MS: time.Since(start).Milliseconds()}}
	if err != nil || status != http.StatusOK {
		return result, err
	}
	var resources []struct {
		ID     string   `json:"id"`
		URL    string   `json:"url"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(data, &resources); err != nil {
		return result, err
	}
	for _, resource := range resources {
		if resource.ID != p.config.CloudID {
			continue
		}
		if resource.URL == "" || (result.SiteURL != "" && resource.URL != result.SiteURL) {
			continue
		}
		result.SiteURL = resource.URL
		result.JiraScopes = result.JiraScopes ||
			(slices.Contains(resource.Scopes, "read:jira-work") && slices.Contains(resource.Scopes, "read:jira-user"))
		result.ConfluenceScopes = result.ConfluenceScopes ||
			(slices.Contains(resource.Scopes, "read:space:confluence") &&
				slices.Contains(resource.Scopes, "read:page:confluence") &&
				slices.Contains(resource.Scopes, "read:content-details:confluence"))
	}
	return result, nil
}

func (p provider) inspectPermission(ctx context.Context, access, target string) (permissionResult, error) {
	start := time.Now()
	data, _, status, err := p.request(ctx, http.MethodGet,
		p.jira("/mypermissions?permissions="+_browseProjects+"&"+target), access, nil)
	result := permissionResult{
		requestResult: requestResult{Status: status, MS: time.Since(start).Milliseconds()},
		Target:        target,
	}
	if err != nil || status != http.StatusOK {
		return result, err
	}
	var body struct {
		Permissions map[string]struct {
			Browse *bool `json:"havePermission"` //nolint:tagliatelle // Jira wire format.
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return result, err
	}
	result.Browse = body.Permissions[_browseProjects].Browse
	return result, nil
}
