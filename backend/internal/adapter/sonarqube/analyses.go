package sonarqube

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

var _sha = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

type analysis struct {
	Key      string `json:"key"`
	Date     string `json:"date"`
	Revision string `json:"revision"`
	at       time.Time
}

type analysisPage struct {
	Paging   paging     `json:"paging"`
	Analyses []analysis `json:"analyses"`
}

func (s source) analysisPage(ctx context.Context, project string, index, size, total int) (analysisPage, error) {
	var page analysisPage
	err := s.get(ctx, "/api/project_analyses/search", pageQuery("project", project, index, size), &page)
	if err != nil {
		return page, pageError(err, index)
	}
	if page.Analyses == nil {
		return page, adaptersdk.InvalidResponseError("SonarQube analysis list is missing")
	}
	if err := page.Paging.check(index, size, len(page.Analyses), total); err != nil {
		return page, err
	}
	for i := range page.Analyses {
		item := &page.Analyses[i]
		item.at, err = time.Parse(time.RFC3339Nano, item.Date)
		if err != nil {
			item.at, err = time.Parse("2006-01-02T15:04:05.999999999-0700", item.Date)
		}
		if err != nil || item.at.IsZero() || item.Key == "" || !textFits(item.Key, 255) ||
			item.Revision != "" && !_sha.MatchString(item.Revision) {
			return page, adaptersdk.InvalidResponseError("invalid SonarQube analysis key, date or revision")
		}
		item.at = item.at.UTC()
	}
	return page, nil
}

func compareAnalysis(a, b analysis) int {
	if order := a.at.Compare(b.at); order != 0 {
		return order
	}
	return cmp.Compare(a.Key, b.Key)
}

// readAnalyses retains the newest source analysis independently of the period and cursor.
func (s source) readAnalyses(ctx context.Context, project string, period adaptersdk.Period, keys []adaptersdk.ObjectKey, cursor *adaptersdk.Cursor) (selected []analysis, latest analysis, err error) {
	total := -1
	seen := map[string]struct{}{}
	for index := 1; ; index++ {
		page, err := s.analysisPage(ctx, project, index, _pageSize, total)
		if err != nil {
			return nil, analysis{}, err
		}
		total = *page.Paging.Total
		if index == 1 && len(page.Analyses) > 0 {
			latest = page.Analyses[0]
		}
		for _, item := range page.Analyses {
			if _, duplicate := seen[item.Key]; duplicate || item.at.After(latest.at) {
				return nil, analysis{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "SonarQube analyses changed during pagination")
			}
			seen[item.Key] = struct{}{}
			if selectAnalysis(item, project, period, keys, cursor) {
				selected = append(selected, item)
			}
		}
		if len(seen) == total {
			break
		}
	}
	slices.SortFunc(selected, compareAnalysis)
	return selected, latest, nil
}

func selectAnalysis(a analysis, project string, period adaptersdk.Period, keys []adaptersdk.ObjectKey, cursor *adaptersdk.Cursor) bool {
	if cursor != nil && compareAnalysis(a, analysis{at: cursor.ModifiedAt, Key: cursor.NativeId}) <= 0 {
		return false
	}
	if len(keys) != 0 {
		return slices.ContainsFunc(keys, func(key adaptersdk.ObjectKey) bool {
			return key.ObjectKind == adaptersdk.ObjectKeyObjectKindReportFile && key.NativeNs == project && key.NativeId == a.Key
		})
	}
	return (period.From == nil || !a.at.Before(*period.From)) && (period.To == nil || a.at.Before(*period.To))
}

func (s source) confirmLatest(ctx context.Context, project string, latest analysis) error {
	page, err := s.analysisPage(ctx, project, 1, 1, -1)
	if err != nil {
		return err
	}
	if len(page.Analyses) == 0 || page.Analyses[0] != latest {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "latest SonarQube analysis changed during collection")
	}
	return nil
}

func manifestCursor(raw *string) (*adaptersdk.Cursor, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(*raw)
	var cursor adaptersdk.Cursor
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.NativeId == "" ||
		!textFits(cursor.NativeId, 255) || cursor.ModifiedAt.IsZero() {
		return nil, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid SonarQube manifest cursor")
	}
	return &cursor, nil
}

func (a *Adapter) Manifest(ctx context.Context, call adaptersdk.Call, in adaptersdk.ManifestInput) (adaptersdk.ManifestPage, error) {
	ctx, cancel := context.WithTimeout(ctx, adaptersdk.SourceTimeout)
	defer cancel()
	if !validScope(in.Scope) || in.Limit < 1 || in.Limit > 10000 {
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid SonarQube manifest scope or limit")
	}
	out := adaptersdk.ManifestPage{Items: []adaptersdk.ManifestItem{}}
	switch in.SchemaId {
	case adaptersdk.ManifestInputSchemaIdFindings:
	case adaptersdk.ManifestInputSchemaIdJunit, adaptersdk.ManifestInputSchemaIdCoverage:
		zero := 0
		out.TotalCount = &zero
		return out, nil
	default:
		return adaptersdk.ManifestPage{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "schema is not supported by SonarQube")
	}
	cursor, err := manifestCursor(in.Cursor)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	s, err := a.bound(ctx, call)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	project := in.Scope.NativeScopeId
	items, latest, err := s.readAnalyses(ctx, project, in.Period, nil, cursor)
	if err != nil {
		return adaptersdk.ManifestPage{}, err
	}
	for _, item := range items[:min(in.Limit, len(items))] {
		entry := adaptersdk.ManifestItem{RecordKind: "report_file", NativeNs: project, NativeId: item.Key, UpdatedAt: &item.at}
		if item.Key == latest.Key {
			page, err := s.issuePage(ctx, project, 1, 1, -1, false)
			if err != nil {
				return adaptersdk.ManifestPage{}, err
			}
			if err := s.confirmLatest(ctx, project, latest); err != nil {
				return adaptersdk.ManifestPage{}, err
			}
			entry.IssueCount = page.Paging.Total
		}
		out.Items = append(out.Items, entry)
	}
	if len(items) > in.Limit {
		last := items[in.Limit-1]
		raw, err := json.Marshal(adaptersdk.Cursor{NativeId: last.Key, ModifiedAt: last.at})
		if err != nil {
			return adaptersdk.ManifestPage{}, err
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		out.NextCursor = &next
	}
	if cursor == nil {
		out.TotalCount = new(len(items))
	}
	return out, nil
}
