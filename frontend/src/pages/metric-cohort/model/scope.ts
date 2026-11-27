import type { MetricInspection } from '@/entities/metric'
import type { CohortItem } from '@/shared/api'

export function matchesReportSlice(
  item: CohortItem,
  selection: MetricInspection,
) {
  if (item.deleted || item.object.kind !== 'task') return true
  const { group, params } = selection
  const expectedType =
    params.groupBy === 'task_type' && group !== null
      ? group || null
      : params.taskType || undefined
  const expectedSize =
    params.groupBy === 'size_label' && group !== null
      ? group || 'unknown'
      : params.size
  return (
    (expectedType === undefined || (item.taskType ?? null) === expectedType) &&
    (!expectedSize ||
      (expectedSize === 'unknown'
        ? item.size == null
        : item.size === expectedSize))
  )
}
