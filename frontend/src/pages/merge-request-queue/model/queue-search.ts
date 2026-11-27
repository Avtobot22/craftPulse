import type { MergeRequestListParams } from '@/shared/api'
import { pageSizes } from '@/shared/ui'

export const waitGroups = {
  no_reviewer: 'Нет ревьюера',
  awaiting_first_response: 'Ждёт первого ответа человека',
} as const

export type WaitGroup = keyof typeof waitGroups

export const waitSorts = {
  '-wait': 'Сначала самые долгие',
  wait: 'Сначала самые короткие',
  '-opened_at': 'Сначала недавно открытые',
  opened_at: 'Сначала давно открытые',
} as const satisfies Record<NonNullable<MergeRequestListParams['sort']>, string>

type WaitSort = keyof typeof waitSorts

export function readGroup(value: string | null): WaitGroup | undefined {
  if (value === 'no_reviewer' || value === 'awaiting_first_response')
    return value
  return undefined
}

export function readSort(value: string | null): WaitSort {
  if (value === 'wait' || value === 'opened_at' || value === '-opened_at')
    return value
  return '-wait'
}

const legacyGroup = (search: URLSearchParams) =>
  readGroup(search.get('review_wait')) ?? 'no_reviewer'

export function readQueuePage(search: URLSearchParams, kind: WaitGroup) {
  const legacy = legacyGroup(search) === kind
  const cursor =
    search.get(`${kind}_cursor`) ?? (legacy ? search.get('cursor') : null)
  const limit =
    search.get(`${kind}_limit`) ?? (legacy ? search.get('limit') : null)
  return {
    cursor: cursor ?? '',
    limit: pageSizes.find((size) => String(size) === limit) ?? 25,
  }
}

function writePageValue(search: URLSearchParams, name: string, value: string) {
  if (value) search.set(name, value)
  else search.delete(name)
}

export function writeQueuePage(
  search: URLSearchParams,
  kind: WaitGroup,
  cursor: string,
  limit: string,
) {
  writePageValue(search, `${kind}_cursor`, cursor)
  writePageValue(search, `${kind}_limit`, limit === '25' ? '' : limit)
}

export function queueSearchForUpdate(search: URLSearchParams) {
  const next = new URLSearchParams(search)
  if (next.has('cursor') || next.has('limit')) {
    const kind = legacyGroup(next)
    const { cursor, limit } = readQueuePage(next, kind)
    writeQueuePage(next, kind, cursor, String(limit))
    next.delete('cursor')
    next.delete('limit')
  }
  return next
}

export function readQueueHistory(state: unknown, kind: WaitGroup): string[] {
  if (!state || typeof state !== 'object' || !('queueHistory' in state))
    return []
  const history = state.queueHistory
  if (!history || typeof history !== 'object' || !(kind in history)) return []
  const cursors: unknown = (history as Partial<Record<WaitGroup, unknown>>)[
    kind
  ]
  return Array.isArray(cursors) &&
    cursors.every((cursor) => typeof cursor === 'string')
    ? cursors
    : []
}
