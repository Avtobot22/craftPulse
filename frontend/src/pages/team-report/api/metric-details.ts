import {
  InvalidResponse,
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { readNumber, readNullableNumber as nullableNumber } from '@/shared/api'

export interface TaskContribution {
  id: string
  label: string
  sourceUrl: string | null
  taskType: string | null
  size: string | null
  role: 'numerator' | 'denominator' | 'excluded' | 'missing'
  value: number | null
  reason: string | null
}

async function readPages<T>(
  read: (
    cursor: string | undefined,
  ) => Promise<{ items: T[]; next: string | null }>,
) {
  const items: T[] = []
  const seen = new Set<string>()
  let cursor: string | undefined
  do {
    const page = await read(cursor)
    items.push(...page.items)
    cursor = page.next ?? undefined
    if (cursor !== undefined) {
      if (seen.has(cursor)) throw new InvalidResponse()
      seen.add(cursor)
    }
  } while (cursor !== undefined)
  return items
}

function readTask(value: unknown): TaskContribution | null {
  const raw = readRecord(value)
  if (readBoolean(raw.basis_deleted)) return null
  const object = readRecord(raw.object)
  if (object.kind !== 'task') return null
  const role = readEnum(raw.role, [
    'numerator',
    'denominator',
    'excluded',
    'missing',
  ])
  if (role === 'unknown') throw new InvalidResponse()
  const id = readString(object.id)
  return {
    id,
    label:
      readNullableString(object.display_key) ??
      readNullableString(object.title) ??
      id,
    sourceUrl: readNullableString(object.source_url),
    taskType: readNullableString(raw.task_type ?? null),
    size: readNullableString(raw.size_label ?? null),
    role,
    value: nullableNumber(raw.value),
    reason: readNullableString(raw.reason_code),
  }
}

export async function loadTaskContributions(
  api: ApiClient,
  revisionId: string,
  metricId: string,
  pointStart: string,
  filters: {
    projectIds: string[] | undefined
    taskType: string | undefined
    size: string | undefined
  },
  signal: AbortSignal,
) {
  const rows = await readPages(
    async (cursor) =>
      (
        await api.read(
          '/revisions/{revision_id}/cohort',
          (value) => {
            const raw = readRecord(value)
            return {
              items: readArray(raw.data, readTask),
              next: readNullableString(raw.next_cursor),
            }
          },
          {
            pathParams: { revision_id: revisionId },
            query: {
              metric_id: metricId,
              point_start: pointStart,
              project_id: filters.projectIds,
              cursor,
            },
            signal,
          },
        )
      ).data,
  )
  return rows.filter(
    (row): row is TaskContribution =>
      row !== null &&
      (filters.taskType === undefined || row.taskType === filters.taskType) &&
      (filters.size === undefined ||
        row.size === (filters.size === 'unknown' ? null : filters.size)),
  )
}

export function loadTargetVersions(
  api: ApiClient,
  teamId: string,
  signal: AbortSignal,
) {
  return readPages(
    async (cursor) =>
      (
        await api.read(
          '/teams/{team_id}/target-versions',
          (value) => {
            const raw = readRecord(value)
            return {
              items: readArray(raw.data, (value) => {
                const target = readRecord(value)
                return {
                  taskType: readString(target.task_type),
                  size: readString(target.size_label),
                  version: readNumber(target.version_no),
                  duration: readNumber(target.duration_seconds),
                  n: nullableNumber(target.basis_n),
                }
              }),
              next: readNullableString(raw.next_cursor),
            }
          },
          { pathParams: { team_id: teamId }, query: { cursor }, signal },
        )
      ).data,
  )
}
