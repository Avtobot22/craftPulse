export interface CohortPagination {
  limit: 25 | 50 | 100
  cursors: string[]
  page: number
}

export function readCohortPagination(state: unknown): CohortPagination {
  const initial = { limit: 25, cursors: [], page: 0 } satisfies CohortPagination
  if (
    typeof state !== 'object' ||
    state === null ||
    !('cohortPagination' in state)
  )
    return initial
  const saved = state.cohortPagination
  if (
    typeof saved !== 'object' ||
    saved === null ||
    !('limit' in saved) ||
    !('cursors' in saved) ||
    !('page' in saved)
  )
    return initial
  const { limit, cursors, page } = saved
  if (
    (limit !== 25 && limit !== 50 && limit !== 100) ||
    !Array.isArray(cursors) ||
    !cursors.every((cursor): cursor is string => typeof cursor === 'string') ||
    typeof page !== 'number' ||
    !Number.isInteger(page) ||
    page < 0 ||
    page > cursors.length
  )
    return initial
  return { limit, cursors, page }
}
