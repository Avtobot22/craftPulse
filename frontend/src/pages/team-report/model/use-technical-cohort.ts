import { useQuery } from '@tanstack/react-query'
import { InvalidResponse, loadCohort } from '@/shared/api'
import type {
  ApiClient,
  CohortItem,
  CohortQuery,
  ReportSeries,
} from '@/shared/api'
import { readReportParams, technicalReportFilters } from '@/shared/config'
import type { ReportBlockProps } from './metrics'

type VisibleItem = Extract<CohortItem, { deleted: false }>

export function useTechnicalCohort(
  api: ApiClient,
  block: ReportBlockProps,
  series: ReportSeries,
  point: string,
  gaps: boolean,
  search: URLSearchParams,
) {
  const { groupBy, projectIds, report } = block
  const taskType =
    groupBy === 'task_type' ? (series.group?.task_type ?? null) : block.taskType
  const size =
    groupBy === 'size_label'
      ? (series.group?.size_label ?? 'unknown')
      : block.size
  const estimator = series.estimator ?? series.group?.signal
  const query = {
    metric_id: series.metric_id,
    point_start: point,
    ...(estimator ? { estimator } : {}),
    ...(projectIds?.length ? { project_id: projectIds } : {}),
    ...(taskType ? { task_type: taskType } : {}),
    ...(size ? { size_label: size } : {}),
    ...(series.metric_id === 'X03'
      ? technicalReportFilters(readReportParams(search))
      : {}),
    ...(!gaps ? { role: 'numerator' } : {}),
    limit: 100,
  } satisfies CohortQuery
  const revisionId = report.revision.id
  return useQuery({
    queryKey: ['technical-cohort', revisionId, query, taskType],
    refetchOnMount: 'always',
    queryFn: async ({ signal }) => {
      let cursor: string | undefined
      let limited = false
      const seen = new Set<string>()
      const items: VisibleItem[] = []
      do {
        const page = await loadCohort(
          api,
          revisionId,
          { ...query, ...(cursor ? { cursor } : {}) },
          signal,
        )
        limited ||= page.accessLimited || page.accessNotes.length > 0
        for (const item of page.items) {
          if (item.deleted) limited = true
          else if (
            (gaps
              ? item.role === 'denominator' || item.role === 'missing'
              : item.role === 'numerator') &&
            (taskType !== null ||
              item.object.kind !== 'task' ||
              item.taskType == null)
          )
            items.push(item)
        }
        cursor = page.next ?? undefined
        if (cursor) {
          if (seen.has(cursor)) throw new InvalidResponse()
          seen.add(cursor)
        }
      } while (cursor)
      return { items, limited }
    },
  })
}
