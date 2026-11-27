package sonarqube

import (
	"cmp"
	"context"
	"maps"
	"math"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var _windowsDrive = regexp.MustCompile(`^[A-Za-z]:`)

//nolint:tagliatelle // SonarQube Web API fields.
type issue struct {
	Key         string `json:"key"`
	Rule        string `json:"rule"`
	Project     string `json:"project"`
	Component   string `json:"component"`
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	IssueStatus string `json:"issueStatus"`
	Resolution  string `json:"resolution"`
	Updated     string `json:"updateDate"`
	Line        int    `json:"line"`
	TextRange   struct {
		Start int `json:"startLine"`
		End   int `json:"endLine"`
	} `json:"textRange"`
}

type component struct {
	Key       string `json:"key"`
	Path      string `json:"path"`
	Qualifier string `json:"qualifier"`
}

type rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Lang string `json:"lang"`
	Repo string `json:"repo"`
}

type issuePage struct {
	Paging     paging      `json:"paging"`
	Issues     []issue     `json:"issues"`
	Components []component `json:"components"`
	Rules      []rule      `json:"rules"`
}

func (s source) issuePage(ctx context.Context, project string, index, size, total int, withRules bool) (issuePage, error) {
	query := pageQuery("components", project, index, size)
	query.Set("s", "UPDATE_DATE")
	query.Set("asc", "false")
	if withRules {
		query.Set("additionalFields", "rules")
	}
	var page issuePage
	err := s.get(ctx, "/api/issues/search", query, &page)
	if err != nil {
		return page, pageError(err, index)
	}
	if page.Issues == nil {
		return page, adaptersdk.InvalidResponseError("SonarQube issue list is missing")
	}
	if err := page.Paging.check(index, size, len(page.Issues), total); err != nil {
		return page, err
	}
	for i := range page.Issues {
		if page.Issues[i].Project != project {
			return page, adaptersdk.InvalidResponseError("SonarQube returned an issue outside the project")
		}
	}
	return page, nil
}

type locatedIssue struct {
	issue issue
	file  string
}

type issueSnapshot struct {
	issues map[string]locatedIssue
	rules  map[string]rule
	total  int
}

func (s source) scanIssues(ctx context.Context, project string, withRules bool) (issueSnapshot, error) {
	out := issueSnapshot{issues: map[string]locatedIssue{}, rules: map[string]rule{}, total: -1}
	for index := 1; ; index++ {
		page, err := s.issuePage(ctx, project, index, _pageSize, out.total, withRules)
		if err != nil {
			return issueSnapshot{}, err
		}
		out.total = *page.Paging.Total
		components := make(map[string]component, len(page.Components))
		for _, item := range page.Components {
			components[item.Key] = item
		}
		for _, item := range page.Rules {
			out.rules[item.Key] = item
		}
		for i := range page.Issues {
			item := &page.Issues[i]
			if _, duplicate := out.issues[item.Key]; duplicate {
				return issueSnapshot{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube issues changed during pagination")
			}
			file, err := item.location(project, components)
			if err != nil {
				return issueSnapshot{}, err
			}
			out.issues[item.Key] = locatedIssue{issue: *item, file: file}
		}
		if index*_pageSize >= min(out.total, _issueWindow) {
			return out, nil
		}
	}
}

func (i issue) location(project string, components map[string]component) (string, error) {
	if !envelope.SonarIssueType(i.Type).Valid() || i.Severity == "" || cmp.Or(i.Status, i.IssueStatus) == "" ||
		i.Key == "" || !textFits(i.Key, 255) || i.Rule == "" || !textFits(i.Rule, 255) ||
		!textFits(i.Type, 64) || !textFits(i.Severity, 64) || !textFits(i.Status, 64) ||
		!textFits(i.IssueStatus, 64) || !textFits(i.Resolution, 64) ||
		i.Line < 0 || i.Line > math.MaxInt32 || i.TextRange.End > math.MaxInt32 || i.TextRange.Start < 0 || i.TextRange.End < i.TextRange.Start ||
		i.TextRange.Start == 0 && i.TextRange.End != 0 {
		return "", adaptersdk.InvalidResponseError("invalid SonarQube issue")
	}
	file := components[i.Component].Path
	if file == "" && i.Component != project {
		// The component key is project:path on the confirmed main-branch endpoint.
		var ok bool
		file, ok = strings.CutPrefix(i.Component, project+":")
		if !ok || file == "" {
			return "", adaptersdk.InvalidResponseError("SonarQube issue component is outside the project")
		}
	}
	if file != "" && (!textFits(file, 4096) || path.IsAbs(file) || _windowsDrive.MatchString(file) ||
		path.Clean(file) != file ||
		file == "." || file == ".." || strings.HasPrefix(file, "../") || strings.ContainsAny(file, "\\\x00")) {
		return "", adaptersdk.InvalidResponseError("invalid SonarQube issue path")
	}
	return file, nil
}

func (s source) currentIssues(ctx context.Context, project string) (issueSnapshot, error) {
	out, err := s.scanIssues(ctx, project, true)
	if err != nil {
		return issueSnapshot{}, err
	}
	// A change can move an unread issue in front of the offset. Require a second matching
	// read for multiple pages, including native state changes that keep the total steady.
	if out.total > _pageSize {
		again, err := s.scanIssues(ctx, project, false)
		if err != nil {
			return issueSnapshot{}, err
		}
		if out.total != again.total || !maps.Equal(out.issues, again.issues) {
			return issueSnapshot{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube issue listing changed during collection")
		}
	}
	for key := range out.issues {
		if _, err := s.resolveRule(ctx, out.rules, out.issues[key].issue.Rule); err != nil {
			return issueSnapshot{}, err
		}
	}
	return out, nil
}

func (s source) resolveRule(ctx context.Context, rules map[string]rule, key string) (rule, error) {
	value := rules[key]
	if value.Name == "" || value.Lang == "" {
		var reply struct {
			Rule rule `json:"rule"`
		}
		if err := s.get(ctx, "/api/rules/show", url.Values{"key": {key}}, &reply); err != nil {
			return rule{}, err
		}
		value = reply.Rule
	}
	if value.Repo == "" {
		value.Repo, _, _ = strings.Cut(key, ":")
	}
	if value.Key != key || value.Name == "" || value.Lang == "" || value.Repo == "" ||
		!textFits(value.Name, 1024) || !textFits(value.Lang, 64) || !textFits(value.Repo, 255) {
		return rule{}, adaptersdk.InvalidResponseError("invalid SonarQube rule")
	}
	rules[key] = value
	return value, nil
}

func (i locatedIssue) reportIssue(r rule) envelope.SonarIssue {
	item := i.issue
	start, end := item.TextRange.Start, item.TextRange.End
	if start == 0 {
		start, end = item.Line, item.Line
	}
	return envelope.SonarIssue{
		IssueId: item.Key, RuleId: item.Rule, Type: envelope.SonarIssueType(item.Type),
		Rule: &envelope.SonarRuleInfo{Key: r.Key, Name: r.Name, Language: r.Lang, Repository: r.Repo},
		Path: optional(i.file), LineStart: positive(start), LineEnd: positive(end),
		Severity: item.Severity, Status: cmp.Or(item.Status, item.IssueStatus), Resolution: optional(item.Resolution),
	}
}

func positive(n int) *int {
	if n > 0 {
		return &n
	}
	return nil
}
