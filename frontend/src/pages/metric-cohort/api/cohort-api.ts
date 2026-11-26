import {
  InvalidResponse,
  readArray,
  readBoolean,
  readNullableNumber,
  readNullableString,
  readNumber,
  readObjectRef,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiClient, operations } from '@/shared/api'

export type CohortQuery =
  operations['listRevisionCohort']['parameters']['query']
export type CohortRole = NonNullable<CohortQuery['role']>

export const roleLabels: Record<CohortRole, string> = {
  numerator: 'Числитель',
  denominator: 'Знаменатель',
  excluded: 'Исключено',
  missing: 'Пропущено',
}

export function parseRole(role: string | null) {
  switch (role) {
    case 'numerator':
    case 'denominator':
    case 'excluded':
    case 'missing':
      return role
    case null:
    default:
      return undefined
  }
}

function readCohortItem(value: unknown) {
  const raw = readRecord(value)
  const itemNo = readNumber(raw.item_no)
  // The tombstone has no displayable object, interval or contribution.
  if (readBoolean(raw.basis_deleted)) return { deleted: true as const, itemNo }

  const role = parseRole(readString(raw.role))
  if (!role) throw new InvalidResponse()
  const interval = raw.interval === undefined ? null : readRecord(raw.interval)
  return {
    deleted: false as const,
    itemNo,
    role,
    value: readNullableNumber(raw.value),
    reason: readNullableString(raw.reason_code),
    object: readObjectRef(raw.object),
    taskType:
      raw.task_type === undefined
        ? undefined
        : readNullableString(raw.task_type),
    size:
      raw.size_label === undefined
        ? undefined
        : readNullableString(raw.size_label),
    interval: interval && {
      from: readString(interval.from),
      to: readNullableString(interval.to),
    },
  }
}

export type CohortItem = ReturnType<typeof readCohortItem>

export async function loadCohort(
  api: ApiClient,
  revisionId: string,
  query: CohortQuery,
  signal: AbortSignal,
) {
  const { data } = await api.read(
    '/revisions/{revision_id}/cohort',
    (value) => {
      const raw = readRecord(value)
      return {
        items: readArray(raw.data, readCohortItem),
        next: readNullableString(raw.next_cursor),
        accessLimited: readBoolean(raw.access_limited),
      }
    },
    { pathParams: { revision_id: revisionId }, query, signal },
  )
  return data
}
