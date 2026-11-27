import { useQuery } from '@tanstack/react-query'
import type { ApiClient, CohortQuery, ReportSeries } from '@/shared/api'
import { loadCrossCohort } from '../api/cross-cohort'
import type { ReportBlockProps } from './metrics'

export function useCrossCohort(
  api: ApiClient,
  block: ReportBlockProps,
  series: ReportSeries,
  point: ReportSeries['points'][number],
) {
  const { projectIds, groupBy, report } = block
  const { group } = series
  const taskType =
    group && groupBy === 'task_type'
      ? (group.task_type ?? null)
      : block.taskType
  const size =
    group && groupBy === 'size_label'
      ? (group.size_label ?? 'unknown')
      : block.size
  const query = {
    metric_id: series.metric_id,
    point_start: point.point_start,
    ...(series.estimator ? { estimator: series.estimator } : {}),
    ...(projectIds?.length ? { project_id: projectIds } : {}),
    ...(taskType ? { task_type: taskType } : {}),
    ...(size ? { size_label: size } : {}),
    limit: 100,
  } satisfies CohortQuery

  const { state, numerator, excluded_count: excluded } = point.value
  const enabled = state !== 'NOT_EVALUATED'
  const contributions = useQuery({
    queryKey: [
      'cross-cohort',
      report.revision.id,
      query,
      taskType,
      numerator,
      excluded,
    ],
    enabled: enabled && ((numerator ?? 0) > 0 || excluded > 0),
    refetchOnMount: 'always',
    queryFn: async ({ signal }) => {
      const [matches, exclusions] = await Promise.all([
        (numerator ?? 0) > 0
          ? loadCrossCohort(
              api,
              report.revision.id,
              { ...query, role: 'numerator' },
              taskType,
              signal,
            )
          : undefined,
        excluded > 0
          ? loadCrossCohort(
              api,
              report.revision.id,
              { ...query, role: 'excluded' },
              taskType,
              signal,
            )
          : undefined,
      ])
      return {
        tasks: matches?.items ?? [],
        withoutMR:
          exclusions?.items.filter(
            ({ reason }) => reason === 'no_merge_request',
          ).length ?? 0,
        limited: matches?.limited === true || exclusions?.limited === true,
      }
    },
  })
  return contributions
}
