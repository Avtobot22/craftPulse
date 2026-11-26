import {
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  InvalidResponse,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'

export function refreshReport(api: ApiClient, seriesId: string) {
  return api.startSeriesRefresh(seriesId, (value) => {
    const raw = readRecord(value)
    const status = readEnum(raw.status, [
      'queued',
      'running',
      'succeeded',
      'failed',
    ])
    if (raw.kind !== 'series_refresh' || status === 'unknown')
      throw new InvalidResponse()
    return status
  })
}

export interface OutcomeFilters {
  projectIds: string[] | undefined
  taskType?: string | null | undefined
  size?: string | null | undefined
}

interface OutcomeReasonPage {
  reasons: (string | null)[]
  next: string | null
}

export async function loadOutcomeReasons(
  api: ApiClient,
  revisionId: string,
  pointStart: string,
  filters: OutcomeFilters,
  signal: AbortSignal,
) {
  const reasons = new Map<string, number>()
  const seen = new Set<string>()
  let cursor: string | null = null
  do {
    const page: OutcomeReasonPage = (
      await api.read<OutcomeReasonPage>(
        '/revisions/{revision_id}/cohort',
        (value) => {
          const raw = readRecord(value)
          return {
            reasons: readArray(raw.data, (item) => {
              const row = readRecord(item)
              if (readBoolean(row.basis_deleted)) return null
              if (
                filters.taskType !== undefined &&
                (row.task_type ?? null) !== filters.taskType
              )
                return null
              if (
                filters.size !== undefined &&
                (row.size_label ?? null) !== filters.size
              )
                return null
              const reason = readNullableString(row.reason_code)
              return reason === 'cancelled' || reason === 'duplicate'
                ? reason
                : 'other'
            }),
            next: readNullableString(raw.next_cursor),
          }
        },
        {
          query: {
            metric_id: 'T07',
            point_start: pointStart,
            role: 'numerator',
            project_id: filters.projectIds,
            cursor: cursor ?? undefined,
          },
          pathParams: { revision_id: revisionId },
          signal,
        },
      )
    ).data
    for (const reason of page.reasons) {
      if (reason) reasons.set(reason, (reasons.get(reason) ?? 0) + 1)
    }
    cursor = page.next
    if (cursor && seen.has(cursor)) throw new InvalidResponse()
    if (cursor) seen.add(cursor)
  } while (cursor)
  return [...reasons]
}
