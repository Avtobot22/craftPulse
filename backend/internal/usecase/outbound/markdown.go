package outbound

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func render(site, orgSlug string, v *entity.SummaryRevision) string {
	link := fmt.Sprintf("%s/merge-requests/%s/summary?revision_no=%d&org_slug=%s", site, v.MRID, v.No, url.QueryEscape(orgSlug))
	var b strings.Builder
	fmt.Fprintf(&b, "**craftPulse · Только предупреждения**\n\nhead_sha: %s\n\n", inline(v.HeadSHA))
	if v.BaseSHA != "" {
		fmt.Fprintf(&b, "base_sha: %s\n\n", inline(v.BaseSHA))
	}
	if !v.BuiltAt.IsZero() {
		fmt.Fprintf(&b, "Сводка собрана %s.\n\n", v.BuiltAt.UTC().Format(time.RFC3339))
	}
	var signals []*entity.SummaryItem
	resolved, restricted := 0, false
	for i := range v.Items {
		item := &v.Items[i]
		if item.RequiredRight == "security" {
			restricted = true
			continue
		}
		switch item.Kind {
		case entity.FindingNew:
			if item.FindingID != nil && item.OccurrenceID != nil {
				signals = append(signals, item)
			}
		case "resolved":
			resolved++
		case "not_evaluated":
			fmt.Fprintf(&b, "- Не проверено: %s (%s)\n", inline(item.AnalysisKind), inline(item.ReasonCode))
		case "tests", "ci_attempts", "coverage":
			renderMetric(&b, item)
		}
	}
	if restricted {
		fmt.Fprintf(&b, "\n[Есть закрытые результаты](%s).\n", link)
	}
	fmt.Fprintf(&b, "\nУстранено сигналов: %d.\n", resolved)
	slices.SortStableFunc(signals, entity.CompareSummarySignals)
	if len(signals) > 0 {
		b.WriteString("\n**Новые сигналы**\n")
	}
	for _, item := range signals[:min(5, len(signals))] {
		var location entity.SummarySignalLocation
		if json.Unmarshal(item.Value, &location) != nil {
			fmt.Fprintf(&b, "- [Сигнал](%s#finding-%s)\n", link, item.FindingID)
			continue
		}
		place := cmp.Or(location.Path, location.Package)
		if location.Line != nil {
			place = fmt.Sprintf("%s:%d", place, *location.Line)
		}
		fmt.Fprintf(&b, "- %s · %s · [сигнал](%s#finding-%s)\n", inline(location.Rule), inline(place), link, item.FindingID)
	}
	fmt.Fprintf(&b, "\n[Полная сводка](%s)\n", link)
	return b.String()
}

func inline(value string) string {
	value = strings.Join(strings.FieldsFunc(value, unicode.IsControl), " ")
	runes := []rune(value)
	value = string(runes[:min(256, len(runes))])
	delimiter := "`"
	for strings.Contains(value, delimiter) {
		delimiter += "`"
	}
	return delimiter + " " + value + " " + delimiter
}

func renderMetric(b *strings.Builder, item *entity.SummaryItem) {
	switch item.Kind {
	case "tests":
		var m entity.TestMetric
		if json.Unmarshal(item.Value, &m) == nil {
			fmt.Fprintf(b, "- Тесты: %s", state(m.State, m.ReasonCode))
			if c := m.Counts; c != nil {
				fmt.Fprintf(b, ", прошло %d, провалено %d, ошибок %d, пропущено %d", c.Passed, c.Failed, c.Errors, c.Skipped)
			}
			b.WriteByte('\n')
		}
	case "ci_attempts":
		var m struct{ Metrics []entity.SeriesPoint }
		if json.Unmarshal(item.Value, &m) == nil {
			for i := range m.Metrics {
				p := &m.Metrics[i]
				if p.MetricID != "I03" {
					continue
				}
				fmt.Fprintf(b, "- CI, смена fail → pass: %s", state(p.State, p.ReasonCode))
				if p.Numerator != nil && p.Denominator != nil {
					fmt.Fprintf(b, ", %.0f из %.0f проверок", *p.Numerator, *p.Denominator)
				}
				b.WriteByte('\n')
			}
		}
	case "coverage":
		renderStructure(b, item.Value)
	}
}

func state(s entity.DataState, reason string) string {
	if reason == "" {
		return inline(string(s))
	}
	return inline(string(s)) + " (" + inline(reason) + ")"
}

func renderStructure(b *strings.Builder, raw json.RawMessage) {
	var m struct {
		Patch entity.Point
		Size  struct {
			State      entity.DataState
			ReasonCode string
			Total      struct{ Files, Added, Deleted int }
		}
		Code []struct {
			Language   string
			Complexity struct {
				State                entity.DataState
				ReasonCode           string
				Deltas, NewFunctions []struct{ Delta *int }
			}
			Clones entity.Point
		}
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	fmt.Fprintf(b, "- C15 Размер изменения: %s, файлов %d, +%d/−%d строк\n",
		state(m.Size.State, m.Size.ReasonCode), m.Size.Total.Files, m.Size.Total.Added, m.Size.Total.Deleted)
	for i := range m.Code {
		c := &m.Code[i]
		label := inline(c.Language)
		increased := 0
		for _, f := range c.Complexity.Deltas {
			if f.Delta != nil && *f.Delta > 0 {
				increased++
			}
		}
		fmt.Fprintf(b, "- C16 Сложность (%s): %s, выросла у %d из %d функций, новых %d\n",
			label, state(c.Complexity.State, c.Complexity.ReasonCode), increased, len(c.Complexity.Deltas), len(c.Complexity.NewFunctions))
		fmt.Fprintf(b, "- C17 Новые дубли (%s): %s\n", label, point(c.Clones))
	}
	label := state(m.Patch.State, m.Patch.ReasonCode)
	if m.Patch.Value != nil {
		label += fmt.Sprintf(", %.2f%%", 100**m.Patch.Value)
	}
	fmt.Fprintf(b, "- Покрытие изменённых строк: %s\n", label)
}

func point(p entity.Point) string {
	label := state(p.State, p.ReasonCode)
	if p.Value != nil {
		label += fmt.Sprintf(", %.2f", *p.Value)
	}
	return label
}
