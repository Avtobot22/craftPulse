package atlassiancheck

import (
	"context"
	"errors"
	"net/http"
)

type accessResult struct {
	Object  string `json:"object"`
	Status  int    `json:"status"`
	Allowed bool   `json:"allowed"`
	Matches bool   `json:"matches"`
}

func (s *Server) check(ctx context.Context, role string) ([]accessResult, error) {
	access, err := s.access(ctx, role)
	if err != nil {
		return nil, err
	}
	id, err := s.provider.account(ctx, access)
	if err != nil {
		return nil, err
	}
	if id != s.config.Accounts[role] {
		return nil, errors.New("stored token identity does not match role")
	}
	all := role == "owner" || role == "collector"
	a := all || role == "a_allowed" || role == "a_denied"
	b := all || role == "b"
	restricted := all || role == "a_allowed"
	objects := []struct {
		name    string
		url     string
		allowed bool
	}{
		{"jira_cpa", s.provider.jira("/project/CPA"), a},
		{"jira_cpb", s.provider.jira("/project/CPB"), a},
		{"jira_cpz", s.provider.jira("/project/CPZ"), b},
		{"jira_restricted", s.provider.jira("/issue/CPA-1?fields=id"), restricted},
		{"confluence_a", s.provider.confluence("/wiki/api/v2/pages/360481?body-format=storage"), a},
		{"confluence_b", s.provider.confluence("/wiki/api/v2/pages/360631?body-format=storage"), b},
		{"confluence_restricted", s.provider.confluence("/wiki/api/v2/pages/491521?body-format=storage"), restricted},
	}
	results := make([]accessResult, 0, len(objects))
	for _, object := range objects {
		_, _, status, requestErr := s.provider.request(ctx, http.MethodGet, object.url, access, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		matches := status == http.StatusOK
		if !object.allowed {
			matches = status == http.StatusForbidden || status == http.StatusNotFound
		}
		results = append(results, accessResult{
			Object: object.name, Status: status, Allowed: object.allowed, Matches: matches,
		})
	}
	return results, nil
}
