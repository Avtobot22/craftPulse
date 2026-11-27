import { useQuery } from '@tanstack/react-query'
import type { ApiClient } from '@/shared/api'
import { loadTaskContributions } from '../api/metric-details'
import type { TaskContribution } from '../api/metric-details'
import type { ReportBlockProps } from './metrics'

export function reportSteps(block: ReportBlockProps, metricIds: string[]) {
  return [
    ...new Set(
      block.report.metrics
        .filter(({ metric_id }) => metricIds.includes(metric_id))
        .flatMap(({ points }) => points.map(({ point_start }) => point_start)),
    ),
  ].sort((a, b) => Date.parse(b) - Date.parse(a))
}

export function useTaskContributions(
  api: ApiClient,
  block: ReportBlockProps,
  metricId: string,
  step: string,
) {
  const { projectIds, taskType, size } = block
  const revision = block.report.revision.id
  const filters = { projectIds, taskType, size }
  return useQuery({
    queryKey: ['team-report', 'task-cohort', revision, metricId, step, filters],
    queryFn: ({ signal }) =>
      loadTaskContributions(api, revision, metricId, step, filters, signal),
    enabled: block.report.metrics.some(
      (series) =>
        series.metric_id === metricId &&
        series.points.some(
          ({ point_start, value }) =>
            point_start === step && value.state !== 'NOT_EVALUATED',
        ),
    ),
  })
}

export function taskSeries(
  block: ReportBlockProps,
  metricId: string,
  step: string,
  task?: TaskContribution,
) {
  return block.report.metrics.find(
    (series) =>
      series.metric_id === metricId &&
      (series.estimator === undefined || series.estimator === 'p50') &&
      series.points.some(({ point_start }) => point_start === step) &&
      (task === undefined ||
        !series.group ||
        (block.groupBy === 'task_type'
          ? (series.group.task_type ?? null) === task.taskType
          : (series.group.size_label ?? null) === task.size)),
  )
}

export function groupLabel(taskType: string | null, size: string | null) {
  return [taskType ?? 'Тип неизвестен', size ?? 'Без размера'].join(' · ')
}
