import { generatePath, useLocation, useNavigate } from 'react-router'
import { routePaths, teamReportReturnState } from '@/shared/config'
import type { ReportBlockProps } from './metrics'
import { seriesKey } from './metrics'
import type { ReportParams, TeamReportScope } from '@/shared/config'

export function useReportPointNavigation(
  search: URLSearchParams,
  groupBy: ReportParams['groupBy'],
  revisionId: string,
  rememberPoint: TeamReportScope['rememberPoint'],
): ReportBlockProps['selectPoint'] {
  const { key } = useLocation()
  const navigate = useNavigate()
  return (series, pointStart, targetId, chartSeries, cohort, destination) => {
    rememberPoint({
      locationKey: key,
      targetId,
      seriesId: seriesKey(series),
      pointStart,
      revisionId,
      ...(chartSeries ? { chartSeries } : {}),
    })
    const details = new URLSearchParams(search)
    details.set('point_start', pointStart)
    details.set('focus_target', targetId)
    details.set('focus_card', seriesKey(series))
    details.delete('focus_series')
    if (chartSeries) details.set('focus_series', chartSeries)
    details.delete('estimator')
    details.delete('group_value')
    const estimator = series.estimator ?? series.group?.signal
    if (estimator) details.set('estimator', estimator)
    if (series.group && groupBy)
      details.set('group_value', series.group[groupBy] ?? '')
    if (cohort) {
      details.set('report_search', search.toString())
      details.set('size_label', cohort.size ?? 'unknown')
      if (cohort.taskType === null) {
        details.delete('task_type')
        details.set('group_by', 'task_type')
        details.set('group_value', '')
      } else {
        if (cohort.taskType !== undefined)
          details.set('task_type', cohort.taskType)
        details.delete('group_by')
        details.delete('group_value')
      }
    }
    if (destination === 'metricCohort') details.delete('point_start')
    void navigate(
      {
        pathname: generatePath(routePaths[destination ?? 'metricPassport'], {
          revisionId,
          metricId: series.metric_id,
          pointStart,
        }),
        search: details.toString(),
      },
      { state: teamReportReturnState },
    )
  }
}
