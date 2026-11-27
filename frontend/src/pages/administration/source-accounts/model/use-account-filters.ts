import { useLocation, useSearchParams } from 'react-router'
import type { SourceAccountListParams } from '@/shared/api'
import { pageSizes } from '@/shared/ui'

export const assignmentFilters = ['assigned', 'unassigned'] as const

export function useAccountFilters() {
  const [search, setSearch] = useSearchParams()
  const state: unknown = useLocation().state
  const assignment = assignmentFilters.find(
    (value) => value === search.get('assignment'),
  )
  const instance = search.get('instance_id') ?? ''
  const cursor = search.get('cursor') ?? ''
  const limit =
    pageSizes.find((size) => String(size) === search.get('limit')) ??
    pageSizes[0]
  const previous =
    cursor &&
    state &&
    typeof state === 'object' &&
    'accountCursors' in state &&
    Array.isArray(state.accountCursors)
      ? state.accountCursors.filter(
          (value: unknown): value is string => typeof value === 'string',
        )
      : []

  const params: SourceAccountListParams = {
    limit,
    ...(assignment ? { assignment } : {}),
    ...(instance ? { instance_id: instance } : {}),
    ...(cursor ? { cursor } : {}),
  }

  function update({
    nextAssignment = assignment ?? '',
    nextInstance = instance,
    nextCursor = '',
    nextLimit = String(limit),
    nextHistory = [],
  }: {
    nextAssignment?: string
    nextInstance?: string
    nextCursor?: string
    nextLimit?: string
    nextHistory?: string[]
  }) {
    const next = new URLSearchParams(search)
    for (const [key, value] of Object.entries({
      assignment: nextAssignment,
      instance_id: nextInstance,
      cursor: nextCursor,
      limit: nextLimit === String(pageSizes[0]) ? '' : nextLimit,
    })) {
      if (value) next.set(key, value)
      else next.delete(key)
    }
    setSearch(next, { state: { accountCursors: nextHistory } })
  }

  return { assignment, instance, cursor, limit, previous, params, update }
}
