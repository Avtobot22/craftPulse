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
  return (series, pointStart, targetId, chartSeries, cohort) => {
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
    details.delete('estimator')
    details.delete('group_value')
    if (series.estimator) details.set('estimator', series.estimator)
    if (series.group && groupBy)
      details.set('group_value', series.group[groupBy] ?? '')
    if (cohort) {
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
    void navigate(
      {
        pathname: generatePath(routePaths.metricPassport, {
          revisionId,
          metricId: series.metric_id,
        }),
        search: details.toString(),
      },
      { state: teamReportReturnState },
    )
  }
}
